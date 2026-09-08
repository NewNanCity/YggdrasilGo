package migrationplan

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func Save(path string, plan Plan) error {
	if err := plan.Validate(); err != nil {
		return err
	}
	if path == "" {
		return errors.New("plan path is required")
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create private plan directory: %w", err)
	}
	temporary, err := os.CreateTemp(dir, ".shared-auth-plan-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return errors.New("refusing to overwrite an existing migration plan")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func Load(path string) (Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Plan{}, err
	}
	var plan Plan
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return Plan{}, err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return Plan{}, errors.New("migration plan contains trailing JSON values")
	}
	if err := plan.Validate(); err != nil {
		return Plan{}, err
	}
	return plan, nil
}
