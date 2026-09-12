package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/artumarinn/arg0s/internal/config"
	"github.com/artumarinn/arg0s/internal/core"
	"github.com/artumarinn/arg0s/internal/orchestrator/strategies"
	"github.com/artumarinn/arg0s/internal/storage"
	"github.com/artumarinn/arg0s/internal/telemetry"
)

func newRunCmd() *cobra.Command {
	var modelFlag string
	var stream bool
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "run <prompt>",
		Short: "Ejecuta un prompt contra un modelo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTask(cmd, args[0], modelFlag, stream, jsonOut)
		},
		SilenceUsage: true,
	}
	cmd.Flags().StringVar(&modelFlag, "model", "", "modelo a usar (vacío = el router lo elige)")
	cmd.Flags().BoolVar(&stream, "stream", false, "muestra la respuesta en streaming")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "salida estructurada en JSON")
	cmd.AddCommand(newRunShowCmd())
	return cmd
}

// providerNameFor resuelve el provider de un modelo (alias o real) para
// persistir en model_runs.provider.
func providerNameFor(models *config.ModelsFile, modelID string) string {
	real := models.ResolveModel(modelID)
	if mc, ok := models.Models[real]; ok {
		return mc.Provider
	}
	return ""
}

func statusFor(err error) string {
	if err != nil {
		return "failed"
	}
	return "ok"
}

func runTask(cmd *cobra.Command, prompt, modelID string, stream, jsonOut bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	models, err := config.LoadModels()
	if err != nil {
		return err
	}
	home, err := config.Home()
	if err != nil {
		return err
	}
	db, err := storage.Open(filepath.Join(home, "arg0s.db"))
	if err != nil {
		return err
	}
	defer db.Close()

	bus := telemetry.NewBus(db.DB)
	exec := buildExecutor(cfg, models, bus)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	runID := storage.NewRunID()

	var profileJSON, routingJSON string
	if modelID == "" {
		decision, err := buildRouter(cfg, models, db, exec).Decide(ctx, runID, prompt)
		if err != nil {
			return err
		}
		modelID = decision.SelectedModel
		if b, err := json.Marshal(decision.Profile); err == nil {
			profileJSON = string(b)
		}
		if b, err := json.Marshal(decision); err == nil {
			routingJSON = string(b)
		}
	}

	cwd, _ := os.Getwd()
	sessionID := storage.NewSessionID()
	if err := db.InsertSession(context.Background(), sessionID, cwd); err != nil {
		return err
	}

	fallback := cfg.Roles.Generator.Fallback

	if stream {
		return runStreaming(cmd, ctx, db, cfg, models, runID, sessionID, prompt, modelID, fallback, profileJSON, routingJSON, jsonOut)
	}

	strat := strategies.NewDirect(exec, modelID, fallback)
	task := &core.Task{ID: core.TaskID(runID), SessionID: core.SessionID(sessionID), Prompt: prompt, CreatedAt: time.Now()}
	result, runErr := strat.Run(ctx, task)

	status := "completed"
	if runErr != nil {
		status = "failed"
		if errors.Is(ctx.Err(), context.Canceled) {
			status = "cancelled"
		}
	}

	persistRun(db, storage.Run{
		ID: runID, SessionID: sessionID, Prompt: prompt, Strategy: string(result.Strategy),
		Profile: profileJSON, Routing: routingJSON,
		Status: status, InputTokens: result.Usage.InputTokens, OutputTokens: result.Usage.OutputTokens,
		CostUSD: result.Usage.CostUSD, StartedAt: result.StartedAt, EndedAt: result.EndedAt,
		Error: errString(runErr),
	}, result.ModelRuns, models, cmd)

	if runErr != nil {
		if status == "cancelled" {
			fmt.Fprintln(cmd.ErrOrStderr(), "cancelado")
			return nil
		}
		return runErr
	}

	if !jsonOut {
		for _, mr := range result.ModelRuns {
			printFallbackWarning(cmd.OutOrStdout(), mr)
		}
	}

	return printResult(cmd, runID, result.Content, result.Usage, jsonOut)
}

func runStreaming(cmd *cobra.Command, ctx context.Context, db *storage.DB, cfg *config.Config, models *config.ModelsFile,
	runID, sessionID, prompt, modelID, fallback, profileJSON, routingJSON string, jsonOut bool) error {

	bus := telemetry.NewBus(db.DB)
	exec := buildExecutor(cfg, models, bus)

	started := time.Now()
	req := core.Request{ModelID: modelID, Prompt: prompt, Role: core.RoleGenerator, Fallback: fallback}
	ch, run, err := exec.Stream(ctx, req)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	var content string
	for c := range ch {
		if c.Err != nil {
			continue // el error final está en run.Err una vez que el canal cierra
		}
		content += c.Delta
		if !jsonOut {
			fmt.Fprint(out, c.Delta)
		}
	}
	if !jsonOut {
		fmt.Fprintln(out)
		printFallbackWarning(out, *run) // seguro leer run acá: el canal ya cerró (ver comentario en Stream)
	}

	status := "completed"
	if run.Err != nil {
		status = "failed"
		if errors.Is(ctx.Err(), context.Canceled) {
			status = "cancelled"
		}
	}

	persistRun(db, storage.Run{
		ID: runID, SessionID: sessionID, Prompt: prompt, Strategy: "direct",
		Profile: profileJSON, Routing: routingJSON,
		Status: status, InputTokens: run.Usage.InputTokens, OutputTokens: run.Usage.OutputTokens,
		CostUSD: run.Usage.CostUSD, StartedAt: started, EndedAt: started.Add(run.Latency),
		Error: errString(run.Err),
	}, []core.ModelRun{*run}, models, cmd)

	if run.Err != nil {
		if status == "cancelled" {
			fmt.Fprintln(cmd.ErrOrStderr(), "cancelado")
			return nil
		}
		return run.Err
	}
	if jsonOut {
		return printResult(cmd, runID, content, run.Usage, true)
	}
	return nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func persistRun(db *storage.DB, run storage.Run, modelRuns []core.ModelRun, models *config.ModelsFile, cmd *cobra.Command) {
	// Persistimos con Background, no con el ctx de la task -- si el
	// usuario canceló con ctrl+c, igual queremos guardar el run como
	// cancelled en vez de perderlo.
	ctx := context.Background()
	if err := db.InsertRun(ctx, run); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: no se pudo persistir el run: %v\n", err)
		return
	}
	for _, mr := range modelRuns {
		row := storage.ModelRunRow{
			ID: storage.NewModelRunID(), RunID: run.ID, ModelID: mr.ModelID,
			Provider: providerNameFor(models, mr.ModelID), Role: string(mr.Role),
			InputTokens: mr.Usage.InputTokens, OutputTokens: mr.Usage.OutputTokens, CostUSD: mr.Usage.CostUSD,
			LatencyMS: mr.Latency.Milliseconds(), Attempts: mr.Attempts, FellBackFrom: mr.FellBackFrom,
			Status: statusFor(mr.Err), Error: errString(mr.Err), StartedAt: run.StartedAt,
		}
		if err := db.InsertModelRun(ctx, row); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: no se pudo persistir el model_run: %v\n", err)
		}
	}
}

// printFallbackWarning avisa en salida normal (no solo en `run show`)
// cuando un ModelRun terminó en un modelo distinto al pedido -- un
// fallback silencioso es indistinguible de "todo funciona bien" para
// quien nunca revisó `run show`.
func printFallbackWarning(out io.Writer, mr core.ModelRun) {
	if mr.FellBackFrom == "" {
		return
	}
	fmt.Fprintf(out, "⚠ %s no disponible, usando fallback: %s\n", mr.FellBackFrom, mr.ModelID)
}

func printResult(cmd *cobra.Command, runID, content string, usage core.Usage, jsonOut bool) error {
	out := cmd.OutOrStdout()
	if jsonOut {
		enc := json.NewEncoder(out)
		return enc.Encode(map[string]any{
			"run_id": runID, "content": content,
			"input_tokens": usage.InputTokens, "output_tokens": usage.OutputTokens, "cost_usd": usage.CostUSD,
		})
	}
	fmt.Fprintln(out, content)
	return nil
}
