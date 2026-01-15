// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"
	"testing"
	"time"
)

func TestPath_Child(t *testing.T) {
	tests := []struct {
		name     string
		build    func() *Path
		expected string
	}{
		{
			name:     "single segment",
			build:    func() *Path { return NewPath("server") },
			expected: "server",
		},
		{
			name:     "two segments",
			build:    func() *Path { return NewPath("server").Child("port") },
			expected: "server.port",
		},
		{
			name:     "deeply nested",
			build:    func() *Path { return NewPath("server").Child("middlewares").Child("jwt").Child("jwks").Child("url") },
			expected: "server.middlewares.jwt.jwks.url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.build()
			if got := path.String(); got != tt.expected {
				t.Errorf("Path.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestPath_ChildDoesNotMutateParent(t *testing.T) {
	parent := NewPath("server")
	child := parent.Child("port")

	if parent.String() != "server" {
		t.Errorf("parent was mutated: got %q, want %q", parent.String(), "server")
	}
	if child.String() != "server.port" {
		t.Errorf("child incorrect: got %q, want %q", child.String(), "server.port")
	}
}

func TestPath_Index(t *testing.T) {
	tests := []struct {
		name     string
		build    func() *Path
		expected string
	}{
		{
			name:     "index on child",
			build:    func() *Path { return NewPath("jwt").Child("user_types").Index(0) },
			expected: "jwt.user_types[0]",
		},
		{
			name:     "index then child",
			build:    func() *Path { return NewPath("jwt").Child("user_types").Index(0).Child("type") },
			expected: "jwt.user_types[0].type",
		},
		{
			name:     "multiple indices",
			build:    func() *Path { return NewPath("items").Index(0).Child("subitems").Index(2) },
			expected: "items[0].subitems[2]",
		},
		{
			name: "deeply nested with index",
			build: func() *Path {
				return NewPath("server").Child("middlewares").Child("jwt").Child("user_types").Index(1).Child("display_name")
			},
			expected: "server.middlewares.jwt.user_types[1].display_name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.build()
			if got := path.String(); got != tt.expected {
				t.Errorf("Path.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestPath_IndexDoesNotMutateParent(t *testing.T) {
	parent := NewPath("items").Child("list")
	child := parent.Index(5)

	if parent.String() != "items.list" {
		t.Errorf("parent was mutated: got %q, want %q", parent.String(), "items.list")
	}
	if child.String() != "items.list[5]" {
		t.Errorf("child incorrect: got %q, want %q", child.String(), "items.list[5]")
	}
}

func TestValidationErrors_Error(t *testing.T) {
	tests := []struct {
		name     string
		errs     ValidationErrors
		expected string
	}{
		{
			name:     "single error",
			errs:     ValidationErrors{{Field: "server.port", Message: "must be between 1 and 65535"}},
			expected: "- server.port: must be between 1 and 65535",
		},
		{
			name: "multiple errors",
			errs: ValidationErrors{
				{Field: "server.port", Message: "must be between 1 and 65535"},
				{Field: "jwt.issuer", Message: "is required when jwt.enabled=true"},
			},
			expected: "- server.port: must be between 1 and 65535\n- jwt.issuer: is required when jwt.enabled=true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.errs.Error(); got != tt.expected {
				t.Errorf("ValidationErrors.Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestValidationErrors_OrNil(t *testing.T) {
	t.Run("empty returns nil", func(t *testing.T) {
		var errs ValidationErrors
		if errs.OrNil() != nil {
			t.Error("OrNil() should return nil for empty ValidationErrors")
		}
	})

	t.Run("non-empty returns self", func(t *testing.T) {
		errs := ValidationErrors{{Field: "test", Message: "error"}}
		if errs.OrNil() == nil {
			t.Error("OrNil() should return non-nil for non-empty ValidationErrors")
		}
	})
}

func TestRequired(t *testing.T) {
	path := NewPath("jwt").Child("issuer")

	err := Required(path)
	if err.Field != "jwt.issuer" {
		t.Errorf("Field = %q, want %q", err.Field, "jwt.issuer")
	}
	if err.Message != "is required" {
		t.Errorf("Message = %q, want %q", err.Message, "is required")
	}
}

func TestMustBeInRange(t *testing.T) {
	path := NewPath("server").Child("port")

	tests := []struct {
		name    string
		value   int
		min     int
		max     int
		wantErr bool
	}{
		{"below min", 0, 1, 65535, true},
		{"at min", 1, 1, 65535, false},
		{"in range", 8080, 1, 65535, false},
		{"at max", 65535, 1, 65535, false},
		{"above max", 65536, 1, 65535, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MustBeInRange(path, tt.value, tt.min, tt.max)
			if (err != nil) != tt.wantErr {
				t.Errorf("MustBeInRange() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMustBeInRange_Duration(t *testing.T) {
	path := NewPath("server").Child("read_timeout")

	t.Run("valid duration", func(t *testing.T) {
		err := MustBeInRange(path, 15*time.Second, 0, 5*time.Minute)
		if err != nil {
			t.Errorf("MustBeInRange() unexpected error: %v", err)
		}
	})

	t.Run("duration too large", func(t *testing.T) {
		err := MustBeInRange(path, 10*time.Minute, 0, 5*time.Minute)
		if err == nil {
			t.Fatal("MustBeInRange() expected error for duration above max")
		}
		// Verify error message contains formatted durations
		if !strings.Contains(err.Message, "5m0s") {
			t.Errorf("error message should contain formatted duration, got: %s", err.Message)
		}
	})
}

func TestMustBeNonNegative(t *testing.T) {
	path := NewPath("timeout")

	t.Run("positive value", func(t *testing.T) {
		if err := MustBeNonNegative(path, 10); err != nil {
			t.Errorf("MustBeNonNegative() unexpected error: %v", err)
		}
	})

	t.Run("zero value", func(t *testing.T) {
		if err := MustBeNonNegative(path, 0); err != nil {
			t.Errorf("MustBeNonNegative() should allow zero: %v", err)
		}
	})

	t.Run("negative value", func(t *testing.T) {
		if err := MustBeNonNegative(path, -1); err == nil {
			t.Error("MustBeNonNegative() expected error for negative value")
		}
	})
}

func TestMustBeOneOf(t *testing.T) {
	path := NewPath("logging").Child("level")
	allowed := []string{"debug", "info", "warn", "error"}

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid value", "info", false},
		{"another valid", "debug", false},
		{"invalid value", "trace", true},
		{"empty value", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MustBeOneOf(path, tt.value, allowed)
			if (err != nil) != tt.wantErr {
				t.Errorf("MustBeOneOf() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	t.Run("error message lists allowed values", func(t *testing.T) {
		err := MustBeOneOf(path, "invalid", allowed)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Message, "debug, info, warn, error") {
			t.Errorf("error message should list allowed values, got: %s", err.Message)
		}
	})
}

func TestMustNotBeEmpty(t *testing.T) {
	path := NewPath("url")

	t.Run("non-empty", func(t *testing.T) {
		if err := MustNotBeEmpty(path, "https://example.com"); err != nil {
			t.Errorf("MustNotBeEmpty() unexpected error: %v", err)
		}
	})

	t.Run("empty", func(t *testing.T) {
		if err := MustNotBeEmpty(path, ""); err == nil {
			t.Error("MustNotBeEmpty() expected error for empty string")
		}
	})
}
