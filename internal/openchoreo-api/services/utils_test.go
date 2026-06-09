// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func TestExtractValidationError_FromNewInvalid(t *testing.T) {
	errs := field.ErrorList{
		field.Invalid(field.NewPath("spec", "resources").Index(0).Child("template"), "${applied.x}", "undeclared id"),
	}
	err := apierrors.NewInvalid(schema.GroupKind{Group: "openchoreo.dev", Kind: "ResourceType"}, "rt-1", errs)

	got := ExtractValidationError(err)

	require.NotNil(t, got)
	require.Equal(t, http.StatusUnprocessableEntity, got.StatusCode)
	require.Contains(t, got.Msg, "undeclared id")
}

func TestExtractValidationError_ReturnsNilForNotFound(t *testing.T) {
	err := apierrors.NewNotFound(schema.GroupResource{Group: "openchoreo.dev", Resource: "resourcetypes"}, "missing")
	require.Nil(t, ExtractValidationError(err))
}

func TestExtractValidationError_ReturnsNilForGenericError(t *testing.T) {
	require.Nil(t, ExtractValidationError(errors.New("some other error")))
}

func TestExtractValidationError_ReturnsNilForNil(t *testing.T) {
	require.Nil(t, ExtractValidationError(nil))
}

func TestExtractValidationMessage_PrefixesFieldPath(t *testing.T) {
	errs := field.ErrorList{
		field.Invalid(field.NewPath("spec", "outputs").Index(0).Child("value"), "${applied.x}", "undeclared id"),
	}
	err := apierrors.NewInvalid(schema.GroupKind{Group: "openchoreo.dev", Kind: "ResourceType"}, "rt-1", errs)

	msg := ExtractValidationMessage(err)

	require.Contains(t, msg, "spec.outputs[0].value: ")
	require.Contains(t, msg, "undeclared id")
}

func TestExtractValidationMessage_JoinsMultipleFieldErrors(t *testing.T) {
	errs := field.ErrorList{
		field.Invalid(field.NewPath("spec", "foo"), "x", "bad x"),
		field.Invalid(field.NewPath("spec", "bar"), "y", "bad y"),
	}
	err := apierrors.NewInvalid(schema.GroupKind{Group: "openchoreo.dev", Kind: "ResourceType"}, "rt-1", errs)

	msg := ExtractValidationMessage(err)

	require.Contains(t, msg, "spec.foo: ")
	require.Contains(t, msg, "spec.bar: ")
	require.Contains(t, msg, "; ")
}
