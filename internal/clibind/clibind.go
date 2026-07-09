// Package clibind binds a typed input struct to cobra flags and positional
// arguments using struct tags, so a single struct defines parameters for both
// the CLI and the MCP tool (whose JSON schema the MCP SDK derives from the same
// struct). This is the convergence layer: params can't drift between surfaces.
//
// Supported field kinds: string, bool, int, float64.
// Tags:
//   - json:"name"        flag/property name (required; first comma-part used)
//   - jsonschema:"desc"  flag usage text (also the MCP description)
//   - cli:"arg"          positional argument instead of a flag
//   - cli:"flag"         explicit flag (the default for non-arg fields)
//   - default:"v"        default value (parsed to the field's type)
package clibind

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// Register wires the struct pointed to by ptr onto cmd: it adds a cobra flag for
// each flag field, sets cmd.Args to require the positional fields, and returns
// apply, which assigns positional arguments into the struct. Call apply inside
// RunE. Flag fields are bound directly and need no apply step.
func Register(cmd *cobra.Command, ptr any) (apply func(args []string) error, err error) {
	v := reflect.ValueOf(ptr)
	if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
		return nil, fmt.Errorf("clibind: ptr must be a pointer to struct, got %T", ptr)
	}
	sv := v.Elem()
	st := sv.Type()

	var positional []int // field indices, in declaration order

	for i := 0; i < st.NumField(); i++ {
		f := st.Field(i)
		if !f.IsExported() {
			continue
		}
		name := jsonName(f)
		if name == "" || name == "-" {
			continue
		}
		usage := f.Tag.Get("jsonschema")
		def := f.Tag.Get("default")
		fv := sv.Field(i)

		if f.Tag.Get("cli") == "arg" {
			if f.Type.Kind() != reflect.String {
				return nil, fmt.Errorf("clibind: positional %q must be string", name)
			}
			positional = append(positional, i)
			continue
		}

		if err := registerFlag(cmd, fv, name, usage, def); err != nil {
			return nil, fmt.Errorf("clibind: field %q: %w", name, err)
		}
	}

	if len(positional) > 0 {
		cmd.Args = cobra.ExactArgs(len(positional))
	} else {
		cmd.Args = cobra.NoArgs
	}

	apply = func(args []string) error {
		if len(args) != len(positional) {
			return fmt.Errorf("expected %d argument(s), got %d", len(positional), len(args))
		}
		for j, idx := range positional {
			sv.Field(idx).SetString(args[j])
		}
		return nil
	}
	return apply, nil
}

// ApplyDefaults sets every non-positional field still at its Go zero value to
// its `default` tag, parsed to the field's type. It mirrors the defaulting
// cobra does for flags (via registerFlag) so the MCP path — which unmarshals
// JSON straight into the struct with no cobra involved — behaves the same as
// the CLI when a caller omits an optional field, instead of silently passing
// through "" (or 0/false) that most service functions never re-default.
func ApplyDefaults(ptr any) error {
	v := reflect.ValueOf(ptr)
	if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("clibind: ptr must be a pointer to struct, got %T", ptr)
	}
	sv := v.Elem()
	st := sv.Type()

	for i := 0; i < st.NumField(); i++ {
		f := st.Field(i)
		if !f.IsExported() || f.Tag.Get("cli") == "arg" {
			continue
		}
		name := jsonName(f)
		if name == "" || name == "-" {
			continue
		}
		def := f.Tag.Get("default")
		if def == "" {
			continue
		}
		fv := sv.Field(i)
		if !fv.IsZero() {
			continue
		}
		if err := setDefault(fv, def); err != nil {
			return fmt.Errorf("clibind: field %q: %w", name, err)
		}
	}
	return nil
}

func setDefault(fv reflect.Value, def string) error {
	switch fv.Kind() {
	case reflect.String:
		fv.SetString(def)
	case reflect.Bool:
		d, err := parseBool(def)
		if err != nil {
			return err
		}
		fv.SetBool(d)
	case reflect.Int:
		d, err := parseInt(def)
		if err != nil {
			return err
		}
		fv.SetInt(int64(d))
	case reflect.Float64:
		d, err := parseFloat(def)
		if err != nil {
			return err
		}
		fv.SetFloat(d)
	default:
		return fmt.Errorf("unsupported kind %s", fv.Kind())
	}
	return nil
}

// registerFlag adds a single cobra flag bound to the struct field fv.
func registerFlag(cmd *cobra.Command, fv reflect.Value, name, usage, def string) error {
	switch fv.Kind() {
	case reflect.String:
		cmd.Flags().StringVar(fv.Addr().Interface().(*string), name, def, usage)
	case reflect.Bool:
		d, err := parseBool(def)
		if err != nil {
			return err
		}
		cmd.Flags().BoolVar(fv.Addr().Interface().(*bool), name, d, usage)
	case reflect.Int:
		d, err := parseInt(def)
		if err != nil {
			return err
		}
		cmd.Flags().IntVar(fv.Addr().Interface().(*int), name, d, usage)
	case reflect.Float64:
		d, err := parseFloat(def)
		if err != nil {
			return err
		}
		cmd.Flags().Float64Var(fv.Addr().Interface().(*float64), name, d, usage)
	default:
		return fmt.Errorf("unsupported kind %s", fv.Kind())
	}
	return nil
}

// jsonName returns the flag/property name from the json tag (or the lowercased
// field name as a fallback).
func jsonName(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag == "" {
		return strings.ToLower(f.Name)
	}
	return strings.Split(tag, ",")[0]
}

func parseBool(s string) (bool, error) {
	if s == "" {
		return false, nil
	}
	return strconv.ParseBool(s)
}

func parseInt(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.Atoi(s)
}

func parseFloat(s string) (float64, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.ParseFloat(s, 64)
}
