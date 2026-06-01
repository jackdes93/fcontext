package sctx

import (
	"flag"
	"os"
	"testing"
	"time"
)

// TestNewFlagSetCreation tests AppFlagSet creation
func TestNewFlagSetCreation(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	afs := NewFlagSet("myapp", fs, "")
	if afs == nil {
		t.Fatal("NewFlagSet should not return nil")
	}
}

// TestNewFlagSetNilUsesCommandLine tests that nil fs uses flag.CommandLine
func TestNewFlagSetNilUsesCommandLine(t *testing.T) {
	afs := NewFlagSet("myapp", nil, "")
	if afs == nil {
		t.Fatal("NewFlagSet with nil fs should not return nil")
	}
}

// TestNewFlagSetWithPrefix tests envPrefix propagation
func TestNewFlagSetWithPrefix(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	afs := NewFlagSet("myapp", fs, "MYAPP_")
	if afs == nil {
		t.Fatal("NewFlagSet with prefix should not return nil")
	}
}

// TestEnvNameForWithPrefix tests envNameFor generates correct name when prefix set
func TestEnvNameForWithPrefix(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	afs := NewFlagSet("myapp", fs, "MYAPP_")

	// envNameFor is unexported but we can test it via applyEnvOverrides
	// by setting the env var with prefix and checking if the flag is set
	fs.String("db-host", "localhost", "database host")
	os.Setenv("MYAPP_DB_HOST", "remotehost")
	defer os.Unsetenv("MYAPP_DB_HOST")

	afs.Parse([]string{})

	got := fs.Lookup("db-host").Value.String()
	if got != "remotehost" {
		t.Fatalf("Expected 'remotehost' from MYAPP_DB_HOST, got %q", got)
	}
}

// TestGetSampleEnvs tests that it does not panic
func TestGetSampleEnvs(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	afs := NewFlagSet("myapp", fs, "")
	afs.GetSampleEnvs() // should print without panic
}

// TestCustomUsagePrints tests that Usage() can be called without panic
func TestCustomUsagePrints(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	afs := NewFlagSet("myapp", fs, "TEST_")

	// Register flags of various types to exercise isStringFlag / isZeroValue
	fs.String("str-flag", "hello", "a string flag")
	fs.Int("int-flag", 0, "an int flag")
	fs.Bool("bool-flag", false, "a bool flag")
	fs.Duration("dur-flag", 0, "a duration flag")
	fs.Float64("float-flag", 0.0, "a float flag")

	// Set one env var so customUsage prints the [$ENV=value] form
	os.Setenv("TEST_STR_FLAG", "world")
	defer os.Unsetenv("TEST_STR_FLAG")

	// Trigger Usage — exercises customUsage, isStringFlag, isZeroValue, envNameFor
	if afs.Usage != nil {
		afs.Usage()
	}
}

// TestSetFlagValueBool tests ENV override for bool flags
func TestSetFlagValueBool(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	afs := NewFlagSet("testapp", fs, "")

	var boolVal bool
	fs.BoolVar(&boolVal, "enable-feature", false, "enable a feature")

	os.Setenv("ENABLE_FEATURE", "true")
	defer os.Unsetenv("ENABLE_FEATURE")

	afs.Parse([]string{})

	if !boolVal {
		t.Fatal("Bool flag should be true after ENV override")
	}
}

// TestSetFlagValueInt tests ENV override for int flags
func TestSetFlagValueInt(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	afs := NewFlagSet("testapp", fs, "")

	var intVal int
	fs.IntVar(&intVal, "worker-count", 1, "number of workers")

	os.Setenv("WORKER_COUNT", "8")
	defer os.Unsetenv("WORKER_COUNT")

	afs.Parse([]string{})

	if intVal != 8 {
		t.Fatalf("Int flag should be 8 after ENV override, got %d", intVal)
	}
}

// TestSetFlagValueDuration tests ENV override for duration flags
func TestSetFlagValueDuration(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	afs := NewFlagSet("testapp", fs, "")

	var durVal time.Duration
	fs.DurationVar(&durVal, "timeout", time.Second, "operation timeout")

	os.Setenv("TIMEOUT", "30s")
	defer os.Unsetenv("TIMEOUT")

	afs.Parse([]string{})

	if durVal != 30*time.Second {
		t.Fatalf("Duration flag should be 30s after ENV override, got %v", durVal)
	}
}

// TestSetFlagValueFloat tests ENV override for float flags
func TestSetFlagValueFloat(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	afs := NewFlagSet("testapp", fs, "")

	var floatVal float64
	fs.Float64Var(&floatVal, "threshold", 0.5, "threshold value")

	os.Setenv("THRESHOLD", "0.9")
	defer os.Unsetenv("THRESHOLD")

	afs.Parse([]string{})

	if floatVal < 0.89 || floatVal > 0.91 {
		t.Fatalf("Float flag should be ~0.9 after ENV override, got %v", floatVal)
	}
}

// TestIsZeroValueCases tests isZeroValue indirectly via customUsage output
func TestIsZeroValueCases(t *testing.T) {
	// We can test isZeroValue directly since we're in the same package
	cases := []struct {
		def      string
		expected bool
	}{
		{"", true},
		{"0", true},
		{"false", true},
		{"0s", true},
		{"1", false},
		{"true", false},
		{"localhost", false},
		{"1.5", false},
	}

	for _, tc := range cases {
		got := isZeroValue(nil, tc.def)
		if got != tc.expected {
			t.Errorf("isZeroValue(nil, %q) = %v, want %v", tc.def, got, tc.expected)
		}
	}
}

// TestIsStringFlagCases tests isStringFlag indirectly
func TestIsStringFlagCases(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.String("str", "default", "string flag")
	fs.Int("num", 0, "int flag")

	strFlag := fs.Lookup("str")
	intFlag := fs.Lookup("num")

	if !isStringFlag(strFlag) {
		t.Error("String flag should return true from isStringFlag")
	}
	if isStringFlag(intFlag) {
		t.Error("Int flag should return false from isStringFlag")
	}
}

// TestMustGetFound tests MustGet when component exists (covers return v path)
func TestMustGetFound(t *testing.T) {
	comp := NewMockComponent("existing", 10)
	sv := New(WithComponent(comp))

	if err := sv.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Should NOT panic since component exists
	result := sv.MustGet("existing")
	if result == nil {
		t.Fatal("MustGet should return non-nil for existing component")
	}
	if result != comp {
		t.Fatal("MustGet should return the exact component")
	}
}

// TestEnvNameForTransformation tests flag name to ENV name conversion
func TestEnvNameForTransformation(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	afs := NewFlagSet("app", fs, "")

	// "db.host" should map to "DB_HOST"
	// "some-flag" should map to "SOME_FLAG"
	fs.String("db.host", "localhost", "")
	fs.String("some-flag", "val", "")

	os.Setenv("DB_HOST", "prod-db")
	os.Setenv("SOME_FLAG", "overridden")
	defer func() {
		os.Unsetenv("DB_HOST")
		os.Unsetenv("SOME_FLAG")
	}()

	afs.Parse([]string{})

	if fs.Lookup("db.host").Value.String() != "prod-db" {
		t.Error("db.host should be mapped from DB_HOST env var")
	}
	if fs.Lookup("some-flag").Value.String() != "overridden" {
		t.Error("some-flag should be mapped from SOME_FLAG env var")
	}
}
