package fcontext

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// AppFlagSet bọc flag.FlagSet + hỗ trợ ENV
type AppFlagSet struct {
	*flag.FlagSet
	appName   string
	envPrefix string // ví dụ: "APP_" => APP_GIN_PORT
}

// NewFlagSet tạo AppFlagSet. Nếu fs == nil -> dùng flag.CommandLine
func NewFlagSet(appName string, fs *flag.FlagSet, envPrefix string) *AppFlagSet {
	if fs == nil {
		fs = flag.CommandLine
	}
	a := &AppFlagSet{
		FlagSet:   fs,
		appName:   appName,
		envPrefix: envPrefix,
	}
	a.Usage = a.customUsage()
	return a
}

// Parse: apply ENV → flag trước, rồi parse args.
// args thường để []string{} nếu bạn muốn bỏ qua os.Args.
func (a *AppFlagSet) Parse(args []string) {
	a.applyEnvOverrides()
	_ = a.FlagSet.Parse(args)
}

// GetSampleEnvs: in gợi ý biến ENV phổ biến
func (a *AppFlagSet) GetSampleEnvs() {
	fmt.Println("# Sample ENVs")
	fmt.Println("APP_ENV=dev           # dev|stg|prd")
	fmt.Println("ENV_FILE=.env         # đường dẫn .env")
}

// ====== internal ======

func (a *AppFlagSet) customUsage() func() {
	return func() {
		_, _ = fmt.Fprintf(os.Stderr, "Usage of %s:\n", a.appName)
		a.VisitAll(func(f *flag.Flag) {
			line := fmt.Sprintf("  -%s", f.Name)
			name, usage := flag.UnquoteUsage(f)
			if name != "" {
				line += " " + name
			}
			if len(line) <= 4 {
				line += "\t"
			} else {
				line += "\n    \t"
			}
			line += usage

			// default value
			if !isZeroValue(f, f.DefValue) {
				if isStringFlag(f) {
					line += fmt.Sprintf(" (default %q)", f.DefValue)
				} else {
					line += fmt.Sprintf(" (default %v)", f.DefValue)
				}
			}

			// ENV name + current ENV (nếu có)
			envName := a.envNameFor(f.Name)
			if v, ok := os.LookupEnv(envName); ok && strings.TrimSpace(v) != "" {
				line += fmt.Sprintf(" [$%s=%q]", envName, v)
			} else {
				line += fmt.Sprintf(" [$%s]", envName)
			}

			_, _ = fmt.Fprintln(os.Stderr, line)
		})
	}
}

func (a *AppFlagSet) applyEnvOverrides() {
	a.VisitAll(func(f *flag.Flag) {
		envName := a.envNameFor(f.Name)
		envVal, ok := os.LookupEnv(envName)
		if !ok || strings.TrimSpace(envVal) == "" {
			return
		}
		// Set theo kiểu flag
		_ = setFlagValue(a.FlagSet, f, envVal)
	})
}

func (a *AppFlagSet) envNameFor(name string) string {
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ToUpper(name)
	if a.envPrefix != "" {
		return a.envPrefix + name
	}
	return name
}

// ====== helpers ======

func isStringFlag(f *flag.Flag) bool {
	// best-effort: đoán string flag qua format
	// không dựa vào reflect để tránh panic
	tv := fmt.Sprintf("%T", f.Value)
	return strings.Contains(tv, "string") || strings.Contains(tv, "String")
}

func isZeroValue(f *flag.Flag, def string) bool {
	// Xem def value “coi như zero” với một số kiểu phổ biến
	if def == "" || def == "0" || def == "false" {
		return true
	}
	// Nếu là duration: zero là "0"
	if d, err := time.ParseDuration(def); err == nil {
		return d == 0
	}
	// Nếu là số nguyên
	if i, err := strconv.ParseInt(def, 10, 64); err == nil {
		return i == 0
	}
	// Nếu là số thực
	if f64, err := strconv.ParseFloat(def, 64); err == nil {
		return f64 == 0
	}
	return false
}

// setFlagValue cố gắng parse theo kiểu phổ biến; cuối cùng fallback Set(raw)
func setFlagValue(fs *flag.FlagSet, f *flag.Flag, raw string) error {
	// bool
	if _, ok := fs.Lookup(f.Name).Value.(interface {
		IsBoolFlag() bool
	}); ok {
		if raw == "" {
			raw = "true"
		}
		return fs.Set(f.Name, raw)
	}
	// duration
	if _, err := time.ParseDuration(raw); err == nil && looksLikeDurationFlag(f) {
		return fs.Set(f.Name, raw)
	}
	// int
	if _, err := strconv.ParseInt(raw, 10, 64); err == nil && looksLikeIntFlag(f) {
		return fs.Set(f.Name, raw)
	}
	// float
	if _, err := strconv.ParseFloat(raw, 64); err == nil && looksLikeFloatFlag(f) {
		return fs.Set(f.Name, raw)
	}
	// string & custom flag.Value
	return fs.Set(f.Name, raw)
}

// heuristics nhận diện kiểu flag (tránh reflect)
func looksLikeDurationFlag(f *flag.Flag) bool {
	tv := fmt.Sprintf("%T", f.Value)
	return strings.Contains(tv, "Duration")
}
func looksLikeIntFlag(f *flag.Flag) bool {
	tv := fmt.Sprintf("%T", f.Value)
	return strings.Contains(tv, "Int") || strings.Contains(tv, "int")
}
func looksLikeFloatFlag(f *flag.Flag) bool {
	tv := fmt.Sprintf("%T", f.Value)
	return strings.Contains(tv, "Float") || strings.Contains(tv, "float")
}
