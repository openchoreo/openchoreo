// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package models

import (
	"testing"
)

// ---- SuccessResponse ----

func TestSuccessResponse(t *testing.T) {
	t.Run("String data", func(t *testing.T) {
		resp := SuccessResponse("hello")
		if !resp.Success {
			t.Errorf("Success = false, want true")
		}
		if resp.Data != "hello" {
			t.Errorf("Data = %v, want %v", resp.Data, "hello")
		}
		if resp.Error != "" {
			t.Errorf("Error = %q, want empty", resp.Error)
		}
		if resp.Code != "" {
			t.Errorf("Code = %q, want empty", resp.Code)
		}
	})

	t.Run("Struct data", func(t *testing.T) {
		data := ProjectResponse{Name: "my-proj", UID: "uid-1"}
		resp := SuccessResponse(data)
		if !resp.Success {
			t.Errorf("Success = false, want true")
		}
		if resp.Data.Name != "my-proj" {
			t.Errorf("Data.Name = %q, want %q", resp.Data.Name, "my-proj")
		}
		if resp.Data.UID != "uid-1" {
			t.Errorf("Data.UID = %q, want %q", resp.Data.UID, "uid-1")
		}
	})

	t.Run("Integer data", func(t *testing.T) {
		resp := SuccessResponse(42)
		if !resp.Success {
			t.Errorf("Success = false, want true")
		}
		if resp.Data != 42 {
			t.Errorf("Data = %v, want 42", resp.Data)
		}
	})

	t.Run("Nil pointer data", func(t *testing.T) {
		var p *ProjectResponse
		resp := SuccessResponse(p)
		if !resp.Success {
			t.Errorf("Success = false, want true")
		}
		if resp.Data != nil {
			t.Errorf("Data = %v, want nil", resp.Data)
		}
	})
}

// ---- ListSuccessResponse ----

func TestListSuccessResponse(t *testing.T) {
	t.Run("Non-empty list with pagination", func(t *testing.T) {
		items := []string{"a", "b", "c"}
		resp := ListSuccessResponse(items, 10, 2, 3)

		if !resp.Success {
			t.Errorf("Success = false, want true")
		}
		if len(resp.Data.Items) != 3 {
			t.Errorf("len(Items) = %d, want 3", len(resp.Data.Items))
		}
		if resp.Data.TotalCount != 10 {
			t.Errorf("TotalCount = %d, want 10", resp.Data.TotalCount)
		}
		if resp.Data.Page != 2 {
			t.Errorf("Page = %d, want 2", resp.Data.Page)
		}
		if resp.Data.PageSize != 3 {
			t.Errorf("PageSize = %d, want 3", resp.Data.PageSize)
		}
		if resp.Error != "" {
			t.Errorf("Error = %q, want empty", resp.Error)
		}
	})

	t.Run("Empty list returns empty items not nil", func(t *testing.T) {
		resp := ListSuccessResponse([]int{}, 0, 1, 20)

		if !resp.Success {
			t.Errorf("Success = false, want true")
		}
		if resp.Data.Items == nil {
			t.Errorf("Items is nil, want empty slice")
		}
		if len(resp.Data.Items) != 0 {
			t.Errorf("len(Items) = %d, want 0", len(resp.Data.Items))
		}
		if resp.Data.TotalCount != 0 {
			t.Errorf("TotalCount = %d, want 0", resp.Data.TotalCount)
		}
	})

	t.Run("Struct slice preserves element values", func(t *testing.T) {
		items := []NamespaceResponse{
			{Name: "ns-a"},
			{Name: "ns-b"},
		}
		resp := ListSuccessResponse(items, 2, 1, 10)

		if resp.Data.Items[0].Name != "ns-a" {
			t.Errorf("Items[0].Name = %q, want %q", resp.Data.Items[0].Name, "ns-a")
		}
		if resp.Data.Items[1].Name != "ns-b" {
			t.Errorf("Items[1].Name = %q, want %q", resp.Data.Items[1].Name, "ns-b")
		}
	})
}

// ---- CursorListSuccessResponse ----

func TestCursorListSuccessResponse(t *testing.T) {
	t.Run("With next cursor and remaining count", func(t *testing.T) {
		items := []string{"x", "y"}
		remaining := int64(5)
		resp := CursorListSuccessResponse(items, "cursor-abc", &remaining)

		if !resp.Success {
			t.Errorf("Success = false, want true")
		}
		if len(resp.Data.Items) != 2 {
			t.Errorf("len(Items) = %d, want 2", len(resp.Data.Items))
		}
		if resp.Data.Pagination.NextCursor != "cursor-abc" {
			t.Errorf("NextCursor = %q, want %q", resp.Data.Pagination.NextCursor, "cursor-abc")
		}
		if resp.Data.Pagination.RemainingCount == nil {
			t.Fatalf("RemainingCount is nil, want non-nil")
		}
		if *resp.Data.Pagination.RemainingCount != 5 {
			t.Errorf("RemainingCount = %d, want 5", *resp.Data.Pagination.RemainingCount)
		}
	})

	t.Run("Last page: empty cursor and nil remaining count", func(t *testing.T) {
		items := []string{"z"}
		resp := CursorListSuccessResponse(items, "", nil)

		if !resp.Success {
			t.Errorf("Success = false, want true")
		}
		if resp.Data.Pagination.NextCursor != "" {
			t.Errorf("NextCursor = %q, want empty (last page)", resp.Data.Pagination.NextCursor)
		}
		if resp.Data.Pagination.RemainingCount != nil {
			t.Errorf("RemainingCount = %v, want nil (last page)", resp.Data.Pagination.RemainingCount)
		}
	})

	t.Run("Empty items with cursor", func(t *testing.T) {
		resp := CursorListSuccessResponse([]string{}, "next-page", nil)

		if !resp.Success {
			t.Errorf("Success = false, want true")
		}
		if len(resp.Data.Items) != 0 {
			t.Errorf("len(Items) = %d, want 0", len(resp.Data.Items))
		}
	})
}

// ---- ErrorResponse ----

func TestErrorResponse(t *testing.T) {
	t.Run("Error with message and code", func(t *testing.T) {
		resp := ErrorResponse("resource not found", "NOT_FOUND")

		if resp.Success {
			t.Errorf("Success = true, want false")
		}
		if resp.Error != "resource not found" {
			t.Errorf("Error = %q, want %q", resp.Error, "resource not found")
		}
		if resp.Code != "NOT_FOUND" {
			t.Errorf("Code = %q, want %q", resp.Code, "NOT_FOUND")
		}
	})

	t.Run("Error with empty code", func(t *testing.T) {
		resp := ErrorResponse("something went wrong", "")

		if resp.Success {
			t.Errorf("Success = true, want false")
		}
		if resp.Error != "something went wrong" {
			t.Errorf("Error = %q, want %q", resp.Error, "something went wrong")
		}
		if resp.Code != "" {
			t.Errorf("Code = %q, want empty", resp.Code)
		}
	})

	t.Run("Empty message and code", func(t *testing.T) {
		resp := ErrorResponse("", "")

		if resp.Success {
			t.Errorf("Success = true, want false")
		}
		if resp.Error != "" {
			t.Errorf("Error = %q, want empty", resp.Error)
		}
	})

	t.Run("Data field is zero value on error", func(t *testing.T) {
		resp := ErrorResponse("bad request", "INVALID")
		if resp.Data != nil {
			t.Errorf("Data = %v, want nil for error response", resp.Data)
		}
	})
}

// ---- BindingStatusType constants ----

// TestBindingStatusTypeConstants verifies that the string values of BindingStatusType
// constants match the documented API contract. A change here would be a breaking API change.
func TestBindingStatusTypeConstants(t *testing.T) {
	cases := []struct {
		constant BindingStatusType
		want     string
	}{
		{BindingStatusTypeInProgress, "InProgress"},
		{BindingStatusTypeReady, "Active"},
		{BindingStatusTypeFailed, "Failed"},
		{BindingStatusTypeSuspended, "Suspended"},
		{BindingStatusTypeUndeployed, "NotYetDeployed"},
	}

	for _, c := range cases {
		if string(c.constant) != c.want {
			t.Errorf("constant value = %q, want %q", string(c.constant), c.want)
		}
	}
}

// ---- BindingReleaseState constants ----

// TestReleaseStateConstants verifies the string values of BindingReleaseState constants.
func TestReleaseStateConstants(t *testing.T) {
	cases := []struct {
		constant BindingReleaseState
		want     string
	}{
		{ReleaseStateActive, "Active"},
		{ReleaseStateUndeploy, "Undeploy"},
	}

	for _, c := range cases {
		if string(c.constant) != c.want {
			t.Errorf("constant value = %q, want %q", string(c.constant), c.want)
		}
	}
}
