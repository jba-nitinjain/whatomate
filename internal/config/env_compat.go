package config

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/knadh/koanf/v2"
)

// Legacy env var support.
//
// Config levels in env vars are separated by a DOUBLE underscore
// (WHATOMATE_DATABASE__HOST -> database.host). Older deployments used a single
// underscore (WHATOMATE_DATABASE_HOST). Those names are still honoured so an
// upgrade never silently drops settings: each legacy name is matched against
// the real config keys (derived from the Config struct's koanf tags), which
// also resolves multi-word keys correctly (WHATOMATE_WHATSAPP_APP_ID ->
// whatsapp.app_id). When both forms are set, the double-underscore form wins.

// legacyEnvKeys maps the single-underscore form of every leaf config key
// (e.g. "database_host") to its dotted koanf key ("database.host"). Forms that
// would be ambiguous between two keys are left out.
func legacyEnvKeys() map[string]string {
	out := map[string]string{}
	ambiguous := map[string]bool{}
	for _, key := range configLeafKeys(reflect.TypeOf(Config{}), "") {
		legacy := strings.ReplaceAll(key, ".", "_")
		if prev, ok := out[legacy]; ok && prev != key {
			ambiguous[legacy] = true
			continue
		}
		out[legacy] = key
	}
	for k := range ambiguous {
		delete(out, k)
	}
	return out
}

func configLeafKeys(t reflect.Type, prefix string) []string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	var keys []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := strings.Split(f.Tag.Get("koanf"), ",")[0]
		if tag == "" || tag == "-" {
			continue
		}
		key := tag
		if prefix != "" {
			key = prefix + "." + tag
		}
		ft := f.Type
		for ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct && ft.PkgPath() == t.PkgPath() {
			keys = append(keys, configLeafKeys(ft, key)...)
			continue
		}
		keys = append(keys, key)
	}
	return keys
}

// loadLegacyEnv applies single-underscore WHATOMATE_* variables that map to a
// known config key. It must run before the double-underscore provider so the
// new form takes precedence. Returns the legacy variable names that were used.
func loadLegacyEnv(k *koanf.Koanf) ([]string, error) {
	known := legacyEnvKeys()
	var used []string
	for _, kv := range os.Environ() {
		name, value, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(name, "WHATOMATE_") || strings.Contains(name, "__") {
			continue
		}
		key, ok := known[strings.ToLower(strings.TrimPrefix(name, "WHATOMATE_"))]
		if !ok {
			continue
		}
		if err := k.Set(key, value); err != nil {
			return nil, err
		}
		used = append(used, name)
	}
	sort.Strings(used)
	return used, nil
}

func warnLegacyEnv(used []string) {
	if len(used) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr,
		"config: deprecated single-underscore env vars in use (%s); rename to the WHATOMATE_SECTION__FIELD form, e.g. WHATOMATE_DATABASE__HOST\n",
		strings.Join(used, ", "))
}
