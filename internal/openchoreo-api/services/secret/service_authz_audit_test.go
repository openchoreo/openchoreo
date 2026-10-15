// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secret_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"

	authzmocks "github.com/openchoreo/openchoreo/internal/authz/core/mocks"
	coreconfig "github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	secretmocks "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/secret/mocks"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

const leakSecretValue = "hunter2-should-not-leak"

func secretWithValue(name, uid string) corev1.Secret {
	return corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace, UID: k8stypes.UID("uid-" + uid)},
		Data:       map[string][]byte{"password": []byte(leakSecretValue)},
	}
}

func auditedCtx() (context.Context, *audit.AuditData) {
	return audit.NewAuditContext(context.Background(), &audit.Resource{}, audit.RequestInfo{})
}

// publishedRecord renders what the emitter would publish for opID.
func publishedRecord(t *testing.T, ctx context.Context, data *audit.AuditData, opID string) string {
	t.Helper()
	policies, errs := audit.NewPolicySet(coreconfig.NewPath("audit"), audit.Settings{Publish: true}, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected policy errors: %v", errs)
	}
	var buf bytes.Buffer
	emitter, err := audit.NewEmitter("openchoreo-api", policies, audit.NewLogger(&buf))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	op := &audit.Operation{ID: opID, Action: "read", ResourceType: "secret", Category: audit.CategoryAccess}
	audit.EmitFromContext(ctx, emitter, "", op, audit.SurfaceREST, audit.ResultSuccess, data, http.Header{}, "")
	return buf.String()
}

func assertNoSecretValue(t *testing.T, record string) {
	t.Helper()
	for _, leak := range []string{leakSecretValue, base64.StdEncoding.EncodeToString([]byte(leakSecretValue))} {
		if strings.Contains(record, leak) {
			t.Errorf("published record contains the secret value %q:\n%s", leak, record)
		}
	}
}

func TestAuditGetSecret_RecordsTheSecretReturned(t *testing.T) {
	s := secretWithValue(testSecret, "a")
	svc := newAuthzService(t, func(m *secretmocks.MockService) {
		m.EXPECT().GetSecret(mock.Anything, testNamespace, testSecret).Return(&s, nil)
	}, newAllowAllPDP(t))

	ctx, data := auditedCtx()
	if _, err := svc.GetSecret(ctx, testNamespace, testSecret); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if data.Returned == nil {
		t.Fatal("Returned = nil, want the secret returned")
	}
	want := audit.Returned{Count: 1, Items: []audit.ReturnedItem{
		{Type: "secret", Namespace: testNamespace, Name: testSecret, UID: "uid-a"},
	}}
	if got := *data.Returned; got.Count != want.Count || len(got.Items) != 1 || got.Items[0] != want.Items[0] {
		t.Errorf("Returned = %+v, want %+v", got, want)
	}

	assertNoSecretValue(t, publishedRecord(t, ctx, data, "GetSecret"))
}

func TestAuditGetSecret_DeniedRecordsNothingReturned(t *testing.T) {
	svc := newAuthzService(t, func(_ *secretmocks.MockService) {}, newDenyAllPDP(t))

	ctx, data := auditedCtx()
	if _, err := svc.GetSecret(ctx, testNamespace, testSecret); err == nil {
		t.Fatal("expected a denial")
	}
	if data.Returned != nil {
		t.Errorf("Returned = %+v, want nothing on a denial", data.Returned)
	}
}

func TestAuditListSecrets_RecordsEverySecretReturned(t *testing.T) {
	inner := &services.ListResult[corev1.Secret]{
		Items: []corev1.Secret{secretWithValue("db-creds", "a"), secretWithValue("api-key", "b")},
	}
	svc := newAuthzService(t, func(m *secretmocks.MockService) {
		m.EXPECT().ListSecrets(mock.Anything, testNamespace, mock.Anything).Return(inner, nil)
	}, listAllowPDP(t))

	ctx, data := auditedCtx()
	if _, err := svc.ListSecrets(ctx, testNamespace, services.ListOptions{Limit: 10}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if data.Returned == nil || data.Returned.Count != 2 || len(data.Returned.Items) != 2 {
		t.Fatalf("Returned = %+v, want two secrets", data.Returned)
	}
	got := *data.Returned
	if got.Items[0].Name != "db-creds" || got.Items[1].Name != "api-key" || got.Items[1].UID != "uid-b" {
		t.Errorf("Items = %+v, want db-creds then api-key with their uids", got.Items)
	}

	assertNoSecretValue(t, publishedRecord(t, ctx, data, "ListSecrets"))
}

// An empty page runs no per-item check, so only the wrapper can replace the
// seeded hierarchy with the list's scope.
func TestAuditListSecrets_RecordsTheListScope(t *testing.T) {
	svc := newAuthzService(t, func(m *secretmocks.MockService) {
		m.EXPECT().ListSecrets(mock.Anything, testNamespace, mock.Anything).
			Return(&services.ListResult[corev1.Secret]{}, nil)
	}, authzmocks.NewMockPDP(t))

	ctx, data := auditedCtx()
	audit.SeedHierarchy(ctx, audit.Hierarchy{Namespace: "claimed", Project: "p1"})
	if _, err := svc.ListSecrets(ctx, testNamespace, services.ListOptions{Limit: 10}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if want := (audit.Hierarchy{Namespace: testNamespace}); data.Hierarchy != want {
		t.Errorf("Hierarchy = %+v, want the list scope %+v", data.Hierarchy, want)
	}
	if data.Returned == nil || data.Returned.Count != 0 {
		t.Errorf("Returned = %+v, want count 0", data.Returned)
	}
}
