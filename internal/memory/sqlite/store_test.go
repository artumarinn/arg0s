package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/artumarinn/arg0s/internal/memory"
	"github.com/artumarinn/arg0s/internal/storage"
)

// openTestStore usa storage.Open (real, no mock) sobre un arg0s.db
// temporal -- el schema de `memories` es dueño de internal/storage, y
// probar contra sqlite real (no un mock de MemoryStore) es justamente
// lo que pidió el checkpoint: es una sola tabla local, no hay excusa
// para mockearla.
func openTestStore(t *testing.T, revalidateAfter time.Duration) *Store {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "arg0s.db"))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return New(db.DB, revalidateAfter)
}

func TestStore_ThenQuery_RoundTrips(t *testing.T) {
	s := openTestStore(t, 0)
	ctx := context.Background()

	m := memory.Memory{
		Project: "arg0s", Category: memory.CategoryDecision,
		Title: "usar sqlite para memoria", Content: "MemoryStore es una interfaz, sqlite es el default",
		Confidence: 0.9, Source: "user", Refs: []string{"docs/ARG0S.md:sec12"},
	}
	if err := s.Store(ctx, m); err != nil {
		t.Fatalf("Store: %v", err)
	}

	got, err := s.Query(ctx, memory.MemoryQuery{Project: "arg0s", Text: "sqlite memoria"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got))
	}
	if got[0].Title != m.Title || got[0].Content != m.Content {
		t.Fatalf("round-trip mismatch: %+v", got[0])
	}
	if len(got[0].Refs) != 1 || got[0].Refs[0] != "docs/ARG0S.md:sec12" {
		t.Fatalf("refs no volvieron: %+v", got[0].Refs)
	}
}

func TestQuery_FiltersByProject(t *testing.T) {
	s := openTestStore(t, 0)
	ctx := context.Background()
	_ = s.Store(ctx, memory.Memory{Project: "arg0s", Category: memory.CategoryFact, Title: "a", Content: "a"})
	_ = s.Store(ctx, memory.Memory{Project: "otro-proyecto", Category: memory.CategoryFact, Title: "b", Content: "b"})

	got, err := s.Query(ctx, memory.MemoryQuery{Project: "arg0s"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 1 || got[0].Title != "a" {
		t.Fatalf("esperaba solo la memoria de arg0s, got %+v", got)
	}
}

func TestQuery_MinRelevance_FiltersOutWeakMatches(t *testing.T) {
	s := openTestStore(t, 0)
	ctx := context.Background()
	_ = s.Store(ctx, memory.Memory{Project: "arg0s", Category: memory.CategoryFact, Title: "TaskProfile", Content: "perfilado del router"})
	_ = s.Store(ctx, memory.Memory{Project: "arg0s", Category: memory.CategoryFact, Title: "algo sin relación", Content: "nada que ver"})

	got, err := s.Query(ctx, memory.MemoryQuery{Project: "arg0s", Text: "TaskProfile router", MinRelevance: 0.3})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 1 || got[0].Title != "TaskProfile" {
		t.Fatalf("esperaba solo el match fuerte, got %+v", got)
	}
}

func TestQuery_Limit_CapsResults(t *testing.T) {
	s := openTestStore(t, 0)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_ = s.Store(ctx, memory.Memory{Project: "arg0s", Category: memory.CategoryFact, Title: "x", Content: "x"})
	}

	got, err := s.Query(ctx, memory.MemoryQuery{Project: "arg0s", Limit: 2})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 (limit), got %d", len(got))
	}
}

func TestQuery_ComputesStale_WithoutPersistingIt(t *testing.T) {
	s := openTestStore(t, time.Millisecond)
	ctx := context.Background()
	m := memory.Memory{
		ID: "mem_old", Project: "arg0s", Category: memory.CategoryLesson, Title: "vieja", Content: "vieja",
		LastVerified: time.Now().Add(-time.Hour),
	}
	if err := s.Store(ctx, m); err != nil {
		t.Fatalf("Store: %v", err)
	}

	// Sin IncludeStale, una vencida (calculada, no persistida) no aparece.
	got, err := s.Query(ctx, memory.MemoryQuery{Project: "arg0s"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("esperaba 0 resultados (stale calculada, sin IncludeStale), got %+v", got)
	}

	// Con IncludeStale, aparece marcada Stale=true en el resultado --
	// pero es un cálculo de Query(), no lo que dice la fila.
	got, err = s.Query(ctx, memory.MemoryQuery{Project: "arg0s", IncludeStale: true})
	if err != nil {
		t.Fatalf("Query IncludeStale: %v", err)
	}
	if len(got) != 1 || !got[0].Stale {
		t.Fatalf("esperaba Stale=true calculado en el resultado, got %+v", got)
	}

	if rowStale := rawStaleColumn(t, s, m.ID); rowStale {
		t.Fatal("Query() escribió stale=1 en la fila -- debe ser puro cálculo, la escritura es de Revalidate()")
	}
}

// TestQuery_100Calls_NeverMutatesTheRow es el test que pidió el
// checkpoint: Query() es lectura pura, sin excepciones -- llamarla
// muchas veces no debe cambiar ni un bit de la fila.
func TestQuery_100Calls_NeverMutatesTheRow(t *testing.T) {
	s := openTestStore(t, time.Millisecond)
	ctx := context.Background()
	m := memory.Memory{
		ID: "mem_fixed", Project: "arg0s", Category: memory.CategoryLesson, Title: "vieja", Content: "vieja",
		LastVerified: time.Now().Add(-time.Hour),
	}
	if err := s.Store(ctx, m); err != nil {
		t.Fatalf("Store: %v", err)
	}

	before := rawRowSnapshot(t, s, m.ID)
	for i := 0; i < 100; i++ {
		if _, err := s.Query(ctx, memory.MemoryQuery{Project: "arg0s", IncludeStale: true}); err != nil {
			t.Fatalf("Query #%d: %v", i, err)
		}
	}
	after := rawRowSnapshot(t, s, m.ID)

	if before != after {
		t.Fatalf("la fila cambió tras 100 Query(): before=%q after=%q", before, after)
	}
}

func TestRevalidate_PersistsStale_ForMemoriesPastRevalidateAfter(t *testing.T) {
	s := openTestStore(t, time.Millisecond)
	ctx := context.Background()
	m := memory.Memory{ID: "mem_old", Project: "arg0s", Category: memory.CategoryLesson, Title: "vieja", Content: "vieja",
		LastVerified: time.Now().Add(-time.Hour)}
	if err := s.Store(ctx, m); err != nil {
		t.Fatalf("Store: %v", err)
	}

	if rawStaleColumn(t, s, m.ID) {
		t.Fatal("no debería estar stale en la fila todavía (Store no lo persiste)")
	}

	marked, err := s.Revalidate(ctx, "arg0s")
	if err != nil {
		t.Fatalf("Revalidate: %v", err)
	}
	if marked != 1 {
		t.Fatalf("marked = %d, want 1", marked)
	}
	if !rawStaleColumn(t, s, m.ID) {
		t.Fatal("Revalidate debería haber persistido stale=1")
	}
}

func rawStaleColumn(t *testing.T, s *Store, id string) bool {
	t.Helper()
	var stale int
	if err := s.db.QueryRow(`SELECT stale FROM memories WHERE id = ?`, id).Scan(&stale); err != nil {
		t.Fatalf("read raw stale: %v", err)
	}
	return stale != 0
}

func rawRowSnapshot(t *testing.T, s *Store, id string) string {
	t.Helper()
	var row string
	err := s.db.QueryRow(`SELECT id || '|' || title || '|' || content || '|' || stale || '|' || last_verified FROM memories WHERE id = ?`, id).Scan(&row)
	if err != nil {
		t.Fatalf("read raw row: %v", err)
	}
	return row
}

func TestForget_DeletesMemory(t *testing.T) {
	s := openTestStore(t, 0)
	ctx := context.Background()
	m := memory.Memory{ID: "mem_fixed", Project: "arg0s", Category: memory.CategoryFact, Title: "borrame", Content: "borrame"}
	if err := s.Store(ctx, m); err != nil {
		t.Fatalf("Store: %v", err)
	}
	if err := s.Forget(ctx, m.ID); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	got, err := s.Query(ctx, memory.MemoryQuery{Project: "arg0s", IncludeStale: true})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("esperaba 0 tras Forget, got %+v", got)
	}
}
