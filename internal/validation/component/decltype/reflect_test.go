// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package decltype

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiservercel "k8s.io/apiserver/pkg/cel"
)

// Test types that exercise various reflection scenarios

type SimpleStruct struct {
	Name      string `json:"name"`
	Count     int    `json:"count"`
	IsEnabled bool   `json:"isEnabled"`
}

type StructWithOptionalFields struct {
	Required string `json:"required"`
	Optional string `json:"optional,omitempty"`
}

type StructWithMapFields struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type StructWithSliceFields struct {
	Commands []string `json:"commands"`
	Args     []string `json:"args,omitempty"`
}

type NestedChild struct {
	Value string `json:"value"`
}

type StructWithNestedStruct struct {
	Child    NestedChild  `json:"child"`
	ChildPtr *NestedChild `json:"childPtr,omitempty"`
}

type StructWithMapOfStructs struct {
	Items map[string]NestedChild `json:"items"`
}

type StructWithSliceOfStructs struct {
	Children []NestedChild `json:"children"`
}

type StructWithPointerField struct {
	Name     string  `json:"name"`
	AliasPtr *string `json:"aliasPtr,omitempty"`
}

type unexportedFieldStruct struct {
	Exported   string `json:"exported"`
	unexported string //nolint:unused // intentionally unexported for testing
}

func Test_fromGoType_SimpleStruct(t *testing.T) {
	declType := fromGoType(reflect.TypeOf(SimpleStruct{}))

	require.NotNil(t, declType)
	assert.Equal(t, "SimpleStruct", declType.TypeName())

	fields := declType.Fields
	assert.Len(t, fields, 3)

	// Verify field types
	assert.Equal(t, apiservercel.StringType, fields["name"].Type)
	assert.Equal(t, apiservercel.IntType, fields["count"].Type)
	assert.Equal(t, apiservercel.BoolType, fields["isEnabled"].Type)

	// All fields should be required (no omitempty)
	assert.True(t, fields["name"].Required)
	assert.True(t, fields["count"].Required)
	assert.True(t, fields["isEnabled"].Required)
}

func Test_fromGoType_OptionalFields(t *testing.T) {
	declType := fromGoType(reflect.TypeOf(StructWithOptionalFields{}))

	require.NotNil(t, declType)
	fields := declType.Fields

	assert.True(t, fields["required"].Required, "field without omitempty should be required")
	assert.False(t, fields["optional"].Required, "field with omitempty should not be required")
}

func Test_fromGoType_MapFields(t *testing.T) {
	declType := fromGoType(reflect.TypeOf(StructWithMapFields{}))

	require.NotNil(t, declType)
	fields := declType.Fields

	labelsField := fields["labels"]
	assert.True(t, labelsField.Type.IsMap(), "labels should be a map")
	assert.Equal(t, apiservercel.StringType, labelsField.Type.KeyType)
	assert.Equal(t, apiservercel.StringType, labelsField.Type.ElemType)
	assert.Equal(t, int64(maxMapSize), labelsField.Type.MaxElements)
}

func Test_fromGoType_SliceFields(t *testing.T) {
	declType := fromGoType(reflect.TypeOf(StructWithSliceFields{}))

	require.NotNil(t, declType)
	fields := declType.Fields

	commandsField := fields["commands"]
	assert.True(t, commandsField.Type.IsList(), "commands should be a list")
	assert.Equal(t, apiservercel.StringType, commandsField.Type.ElemType)
	assert.Equal(t, int64(maxListSize), commandsField.Type.MaxElements)
}

func Test_fromGoType_NestedStruct(t *testing.T) {
	declType := fromGoType(reflect.TypeOf(StructWithNestedStruct{}))

	require.NotNil(t, declType)
	fields := declType.Fields

	// Direct nested struct
	childField := fields["child"]
	assert.True(t, childField.Type.IsObject(), "child should be an object")
	assert.Equal(t, "NestedChild", childField.Type.TypeName())
	assert.Contains(t, childField.Type.Fields, "value")

	// Pointer to struct (should unwrap to the struct type)
	childPtrField := fields["childPtr"]
	assert.True(t, childPtrField.Type.IsObject(), "childPtr should be an object")
	assert.Equal(t, "NestedChild", childPtrField.Type.TypeName())
	assert.False(t, childPtrField.Required, "pointer field with omitempty should not be required")
}

func Test_fromGoType_MapOfStructs(t *testing.T) {
	declType := fromGoType(reflect.TypeOf(StructWithMapOfStructs{}))

	require.NotNil(t, declType)
	fields := declType.Fields

	itemsField := fields["items"]
	assert.True(t, itemsField.Type.IsMap(), "items should be a map")
	assert.Equal(t, apiservercel.StringType, itemsField.Type.KeyType)

	// Value type should be NestedChild struct
	valueType := itemsField.Type.ElemType
	assert.True(t, valueType.IsObject(), "map value should be an object")
	assert.Equal(t, "NestedChild", valueType.TypeName())
	assert.Contains(t, valueType.Fields, "value")
}

func Test_fromGoType_SliceOfStructs(t *testing.T) {
	declType := fromGoType(reflect.TypeOf(StructWithSliceOfStructs{}))

	require.NotNil(t, declType)
	fields := declType.Fields

	childrenField := fields["children"]
	assert.True(t, childrenField.Type.IsList(), "children should be a list")

	// Element type should be NestedChild struct
	elemType := childrenField.Type.ElemType
	assert.True(t, elemType.IsObject(), "list element should be an object")
	assert.Equal(t, "NestedChild", elemType.TypeName())
}

func Test_fromGoType_UnexportedFields(t *testing.T) {
	declType := fromGoType(reflect.TypeOf(unexportedFieldStruct{}))

	require.NotNil(t, declType)
	fields := declType.Fields

	// Only exported field should be present
	assert.Contains(t, fields, "exported")
	assert.NotContains(t, fields, "unexported")
	assert.Len(t, fields, 1)
}

func Test_fromGoType_PrimitiveTypes(t *testing.T) {
	tests := []struct {
		name     string
		goType   reflect.Type
		expected *apiservercel.DeclType
	}{
		{"string", reflect.TypeOf(""), apiservercel.StringType},
		{"int", reflect.TypeOf(int(0)), apiservercel.IntType},
		{"int8", reflect.TypeOf(int8(0)), apiservercel.IntType},
		{"int16", reflect.TypeOf(int16(0)), apiservercel.IntType},
		{"int32", reflect.TypeOf(int32(0)), apiservercel.IntType},
		{"int64", reflect.TypeOf(int64(0)), apiservercel.IntType},
		{"uint", reflect.TypeOf(uint(0)), apiservercel.UintType},
		{"uint8", reflect.TypeOf(uint8(0)), apiservercel.UintType},
		{"uint16", reflect.TypeOf(uint16(0)), apiservercel.UintType},
		{"uint32", reflect.TypeOf(uint32(0)), apiservercel.UintType},
		{"uint64", reflect.TypeOf(uint64(0)), apiservercel.UintType},
		{"float32", reflect.TypeOf(float32(0)), apiservercel.DoubleType},
		{"float64", reflect.TypeOf(float64(0)), apiservercel.DoubleType},
		{"bool", reflect.TypeOf(true), apiservercel.BoolType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			declType := fromGoType(tt.goType)
			assert.Equal(t, tt.expected, declType)
		})
	}
}

func Test_fromGoType_SliceTypes(t *testing.T) {
	tests := []struct {
		name         string
		goType       reflect.Type
		expectedElem *apiservercel.DeclType
	}{
		{"[]string", reflect.TypeOf([]string{}), apiservercel.StringType},
		{"[]int", reflect.TypeOf([]int{}), apiservercel.IntType},
		{"[]bool", reflect.TypeOf([]bool{}), apiservercel.BoolType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			declType := fromGoType(tt.goType)
			assert.True(t, declType.IsList(), "should be a list")
			assert.Equal(t, tt.expectedElem, declType.ElemType)
			assert.Equal(t, int64(maxListSize), declType.MaxElements)
		})
	}
}

func Test_fromGoType_MapTypes(t *testing.T) {
	tests := []struct {
		name         string
		goType       reflect.Type
		expectedKey  *apiservercel.DeclType
		expectedElem *apiservercel.DeclType
	}{
		{"map[string]string", reflect.TypeOf(map[string]string{}), apiservercel.StringType, apiservercel.StringType},
		{"map[string]int", reflect.TypeOf(map[string]int{}), apiservercel.StringType, apiservercel.IntType},
		{"map[string]bool", reflect.TypeOf(map[string]bool{}), apiservercel.StringType, apiservercel.BoolType},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			declType := fromGoType(tt.goType)
			assert.True(t, declType.IsMap(), "should be a map")
			assert.Equal(t, tt.expectedKey, declType.KeyType)
			assert.Equal(t, tt.expectedElem, declType.ElemType)
			assert.Equal(t, int64(maxMapSize), declType.MaxElements)
		})
	}
}

func Test_fromGoType_PointerTypes(t *testing.T) {
	// Pointer to primitive should unwrap
	stringVal := ""
	declType := fromGoType(reflect.TypeOf(&stringVal))
	assert.Equal(t, apiservercel.StringType, declType)

	// Pointer to struct should unwrap
	structVal := SimpleStruct{}
	declType = fromGoType(reflect.TypeOf(&structVal))
	assert.True(t, declType.IsObject())
	assert.Equal(t, "SimpleStruct", declType.TypeName())
}

func Test_fromGoType_InterfaceType(t *testing.T) {
	// interface{} or any should become DynType
	var anyType any
	declType := fromGoType(reflect.TypeOf(&anyType).Elem())
	assert.Equal(t, apiservercel.DynType, declType)
}

func Test_fromGoType_TypeDefinition(t *testing.T) {
	// Type alias for map should work
	type StringMap map[string]string
	declType := fromGoType(reflect.TypeOf(StringMap{}))
	assert.True(t, declType.IsMap())
	assert.Equal(t, apiservercel.StringType, declType.KeyType)
	assert.Equal(t, apiservercel.StringType, declType.ElemType)
}

func TestGetJSONFieldName(t *testing.T) {
	type TestStruct struct {
		NoTag        string
		WithTag      string `json:"customName"`
		OmitEmpty    string `json:"name,omitempty"`
		Ignore       string `json:"-"`
		EmptyTagName string `json:",omitempty"`
	}

	typ := reflect.TypeOf(TestStruct{})

	tests := []struct {
		fieldName string
		expected  string
	}{
		{"NoTag", "NoTag"},
		{"WithTag", "customName"},
		{"OmitEmpty", "name"},
		{"Ignore", "-"},
		{"EmptyTagName", "EmptyTagName"},
	}

	for _, tt := range tests {
		t.Run(tt.fieldName, func(t *testing.T) {
			field, _ := typ.FieldByName(tt.fieldName)
			result := getJSONFieldName(field)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsOmitempty(t *testing.T) {
	type TestStruct struct {
		Required string `json:"required"`
		Optional string `json:"optional,omitempty"`
		MultiTag string `json:"multi,omitempty,string"`
		NoTag    string
	}

	typ := reflect.TypeOf(TestStruct{})

	tests := []struct {
		fieldName string
		expected  bool
	}{
		{"Required", false},
		{"Optional", true},
		{"MultiTag", true},
		{"NoTag", false},
	}

	for _, tt := range tests {
		t.Run(tt.fieldName, func(t *testing.T) {
			field, _ := typ.FieldByName(tt.fieldName)
			result := isOmitempty(field)
			assert.Equal(t, tt.expected, result)
		})
	}
}
