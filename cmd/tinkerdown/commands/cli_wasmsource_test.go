package commands

import (
	"context"
	"strings"
	"testing"

	"github.com/livetemplate/tinkerdown/internal/config"
	"github.com/livetemplate/tinkerdown/internal/source"
)

// TestCLIWasmSourceReadParityAndReadonlyGuard verifies the wasm CLI parity added in
// M5 Phase 3: createCLISource now constructs a wasm source (it previously fell through
// to "unsupported source type for CLI"). It exercises the whole surface that IS
// verifiable in a TinyGo-free environment, against the committed read-only quotes
// module:
//   - the source constructs and Fetches — the read path actually reaches the CLI; and
//   - the readonly guard holds: a module with no `write` export reports IsReadonly()
//     true, and WriteItem refuses with a guard error rather than silently dropping a
//     write.
//
// A *successful* write needs a module that exports `write`, which requires TinyGo to
// build (absent here). The only committed .wasm is read-only, and no test in any
// transport — CLI or WebSocket — exercises a successful write. That gap is tracked as
// a follow-up (see the plan's M5 Phase 3 Learn / #216). Do not "verify" what cannot be
// run.
func TestCLIWasmSourceReadParityAndReadonlyGuard(t *testing.T) {
	const wasmDir = "../../../examples/lvt-source-wasm-test"
	cfg := config.SourceConfig{
		Type:    "wasm",
		Path:    "./sources/quotes.wasm",
		Options: map[string]string{"category": "inspiration"},
	}

	src, err := createCLISource("quotes", cfg, wasmDir, "")
	if err != nil {
		t.Fatalf("createCLISource(wasm) — the parity gap this phase closes: %v", err)
	}
	defer src.Close()

	ctx := context.Background()
	rows, err := src.Fetch(ctx)
	if err != nil {
		t.Fatalf("wasm Fetch (read parity): %v", err)
	}
	if len(rows) == 0 {
		t.Error("wasm source returned no rows — the read path is not actually running")
	}

	writable, ok := src.(source.WritableSource)
	if !ok {
		t.Fatal("wasm source does not implement WritableSource")
	}
	if !writable.IsReadonly() {
		t.Error("the quotes module exports no `write`, so IsReadonly() should report true")
	}
	err = writable.WriteItem(ctx, "add", map[string]interface{}{"text": "x"})
	if err == nil {
		t.Fatal("WriteItem on a read-only module must error, not silently succeed")
	}
	if !strings.Contains(err.Error(), "does not support write") {
		t.Errorf("WriteItem error = %q, want the 'does not support write operations' guard", err)
	}
}
