package adapter

import (
	"os"
	"path/filepath"

	_ "embed"

	"gopkg.in/yaml.v3"
)

//go:embed defaults.yaml
var defaultsYAML []byte

// Adapter describes how to handle a known AI CLI.
type Adapter struct {
	Name     string   `yaml:"name"`
	Bin      string   `yaml:"bin"`
	WhenArgs []string `yaml:"when_args"`
	Inject   []string `yaml:"inject"`
	Renderer string   `yaml:"renderer"`
}

type config struct {
	Adapters []Adapter `yaml:"adapters"`
}

// Load returns the merged adapter list: embedded defaults overlaid with the
// optional user file. User entries with the same name override embedded ones;
// new names extend the set.
func Load() []Adapter {
	def := parse(defaultsYAML)
	userPath := userConfigPath()
	merged := def
	if userPath != "" {
		if data, err := os.ReadFile(userPath); err == nil {
			user := parse(data)
			merged = merge(def, user)
		}
	}
	return merged
}

func parse(data []byte) []Adapter {
	var c config
	_ = yaml.Unmarshal(data, &c)
	return c.Adapters
}

func merge(base, override []Adapter) []Adapter {
	idx := map[string]int{}
	out := make([]Adapter, len(base))
	copy(out, base)
	for i, a := range out {
		idx[a.Name] = i
	}
	for _, a := range override {
		if i, ok := idx[a.Name]; ok {
			out[i] = a
		} else {
			idx[a.Name] = len(out)
			out = append(out, a)
		}
	}
	return out
}

// userConfigPath returns $BG_HOME/adapters.yaml, or ~/.config/bg/adapters.yaml
// when BG_HOME is unset. Returns "" if neither can be resolved.
func userConfigPath() string {
	if bgHome := os.Getenv("BG_HOME"); bgHome != "" {
		return filepath.Join(bgHome, "adapters.yaml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "bg", "adapters.yaml")
}

// Match returns the adapter applicable to the given command + args, or nil.
// bin matches filepath.Base(command). when_args: applies only if at least one
// listed arg appears in userArgs (empty when_args = always applies).
func Match(command string, userArgs []string) *Adapter {
	adapters := Load()
	base := filepath.Base(command)
	for i := range adapters {
		a := adapters[i]
		if a.Bin != base {
			continue
		}
		if len(a.WhenArgs) == 0 {
			return &a
		}
		for _, w := range a.WhenArgs {
			for _, ua := range userArgs {
				if ua == w {
					return &a
				}
			}
		}
	}
	return nil
}

// WriteDefaultUserConfig writes a commented starter config to the user path if
// it does not already exist. Best-effort; errors ignored by callers.
func WriteDefaultUserConfig() error {
	p := userConfigPath()
	if p == "" {
		return nil
	}
	if _, err := os.Stat(p); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(starterConfig), 0o644)
}

const starterConfig = `# bg adapters — user overrides. Entries here override embedded defaults by name.
adapters:
  # - name: pi
  #   bin: pi
  #   when_args: ["-p"]
  #   inject: ["--output-format", "json"]
  #   renderer: claude-stream-json   # or "raw"
`
