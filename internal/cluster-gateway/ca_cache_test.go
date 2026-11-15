// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/cluster-gateway/fabric"
)

// sharedPlaneTenants returns n DataPlanes on plane "prod", one per namespace
// tenant-<i>, each with its client CA in a Secret of its own namespace. This is
// the shape of a deployment with one DataPlane per organization, all pointing
// at one physical data plane.
func sharedPlaneTenants(n int) []client.Object {
	return sharedPlaneTenantsWithCA(n, nil)
}

// sharedPlaneTenantsWithCA is sharedPlaneTenants with ca as every tenant's
// client CA. With a nil ca each tenant gets a placeholder of its own, for tests
// that only follow which bytes were read.
func sharedPlaneTenantsWithCA(n int, ca []byte) []client.Object {
	objects := make([]client.Object, 0, 2*n)
	for i := range n {
		namespace := fmt.Sprintf("tenant-%d", i)
		tenantCA := ca
		if tenantCA == nil {
			tenantCA = []byte("ca of " + namespace)
		}
		objects = append(objects,
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "agent-ca", Namespace: namespace},
				Data:       map[string][]byte{"ca.crt": tenantCA},
			},
			&openchoreov1alpha1.DataPlane{
				ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: namespace},
				Spec: openchoreov1alpha1.DataPlaneSpec{
					PlaneID: "prod",
					ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
						ClientCA: openchoreov1alpha1.ValueFrom{
							SecretKeyRef: &openchoreov1alpha1.SecretKeyReference{Name: "agent-ca", Key: "ca.crt"},
						},
					},
				},
			},
		)
	}
	return objects
}

// apiCalls records what reaches the API server through a countingClient.
type apiCalls struct {
	secretGets  atomic.Int32
	lists       atomic.Int32
	inFlight    atomic.Int32
	maxInFlight atomic.Int32
	// secretDelay is how long a Secret read takes. The read ends early, with
	// the context's error, when its context does.
	secretDelay time.Duration
}

func countingClient(calls *apiCalls, objects ...client.Object) client.Client {
	return fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(objects...).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, isSecret := obj.(*corev1.Secret); isSecret {
					calls.secretGets.Add(1)
					n := calls.inFlight.Add(1)
					defer calls.inFlight.Add(-1)
					for {
						highest := calls.maxInFlight.Load()
						if n <= highest || calls.maxInFlight.CompareAndSwap(highest, n) {
							break
						}
					}
					if calls.secretDelay > 0 {
						select {
						case <-time.After(calls.secretDelay):
						case <-ctx.Done():
							return ctx.Err()
						}
					}
				}
				return c.Get(ctx, key, obj, opts...)
			},
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				calls.lists.Add(1)
				return c.List(ctx, list, opts...)
			},
		}).Build()
}

// The replicas of an agent connect one after another, and each is checked
// against the same CRs. Reading every CR's Secret again for each of them makes
// an agent restart cost replicas times CRs.
func TestConnect_ReusesCAsReadByAnEarlierConnect(t *testing.T) {
	const tenants = 40
	var calls apiCalls
	s := New(&Config{}, countingClient(&calls, sharedPlaneTenants(tenants)...), testLogger())

	first, _, err := s.getAllPlaneClientCAs(planeTypeDataPlane, "prod")
	require.NoError(t, err)
	require.Len(t, first, tenants)
	require.Equal(t, int32(tenants), calls.secretGets.Load())

	second, _, err := s.getAllPlaneClientCAs(planeTypeDataPlane, "prod")
	require.NoError(t, err)
	assert.Equal(t, first, second)
	assert.Equal(t, int32(tenants), calls.secretGets.Load(), "the second connect read Secrets again")
}

// Read one at a time, the Secrets of a few hundred CRs take longer than the
// agent waits for its handshake, and then no agent can connect to the plane.
// The reads have to overlap. They also have to stay within a limit, or one
// connect opens a request to the API server for every CR at once.
func TestConnect_ReadsCASecretsConcurrentlyWithinTheLimit(t *testing.T) {
	const tenants = 4 * caReadConcurrency
	calls := apiCalls{secretDelay: 5 * time.Millisecond}
	s := New(&Config{}, countingClient(&calls, sharedPlaneTenants(tenants)...), testLogger())

	result, _, err := s.getAllPlaneClientCAs(planeTypeDataPlane, "prod")
	require.NoError(t, err)
	require.Len(t, result, tenants)

	assert.Greater(t, calls.maxInFlight.Load(), int32(1), "Secrets were read one at a time")
	assert.LessOrEqual(t, calls.maxInFlight.Load(), int32(caReadConcurrency))
}

// An agent gives up on a connect after its handshake timeout and starts again.
// A lookup that cannot finish in time must not fail the connect, because every
// retry reads the same Secrets and runs out of time the same way, and the plane
// is left with no agent at all. The connect goes ahead with the CRs that were
// read, and reports the rest so they can be authorized once it is registered.
func TestConnect_GoesAheadWithTheCRsReadInTime(t *testing.T) {
	previous := connectLookupTimeout
	connectLookupTimeout = 150 * time.Millisecond
	t.Cleanup(func() { connectLookupTimeout = previous })

	inline := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "inline"},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "prod",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{Value: "inline ca"},
			},
		},
	}
	// Secret reads that never finish stand in for an API server too slow to
	// serve every CR's Secret within the lookup.
	calls := apiCalls{secretDelay: time.Hour}
	s := New(&Config{}, countingClient(&calls, append(sharedPlaneTenants(3*caReadConcurrency), inline)...), testLogger())

	start := time.Now()
	result, unread, err := s.getAllPlaneClientCAs(planeTypeDataPlane, "prod")

	require.NoError(t, err)
	assert.Less(t, time.Since(start), 5*time.Second, "the lookup outlasted its timeout")
	assert.Equal(t, map[string][]byte{"inline/default": []byte("inline ca")}, result)
	assert.Len(t, unread, 3*caReadConcurrency, "every CR left out must be reported")
	assert.Contains(t, unread, "tenant-0/default")
}

// A connect that ran out of lookup time leaves its agent authorized for only
// some of the plane's CRs. Nothing else reads the rest. The controller
// re-notifies a CR only when no agent at all is authorized for it, and a CR
// left out here is often still served by another replica's agent. Unless the
// gateway finishes the job, those CRs run on fewer agents than are connected,
// or on none, until the agent happens to reconnect.
func TestConnect_AuthorizesTheCRsItLeftUnread(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, caCert, caKey)
	var calls apiCalls
	s := New(&Config{}, countingClient(&calls, sharedPlaneTenantsWithCA(3, encodeCertToPEM(t, caCert))...), testLogger())

	// The connection as a partial connect registers it: authorized for one CR.
	_, err := s.connMgr.Register("dataplane", "prod", &mockGatewayConn{}, []string{"tenant-0/default"}, clientCert, nil)
	require.NoError(t, err)

	s.authorizeUnreadCRs("dataplane", "prod", []string{"tenant-1/default", "tenant-2/default"})

	for _, cr := range []string{"tenant-0/default", "tenant-1/default", "tenant-2/default"} {
		_, err := s.connMgr.GetForCR("dataplane/prod", cr)
		assert.NoError(t, err, "%s is not authorized", cr)
	}
}

// A notification for a CR reads its CA from the API server and applies it to
// the connected agents. A connect that follows must be checked against that
// same CA. Served an older cached one, it would authorize an agent the update
// had just revoked, or refuse one it had just granted, and nothing would
// correct it.
func TestConnect_UsesTheCAOfALaterNotification(t *testing.T) {
	var calls apiCalls
	live := countingClient(&calls, sharedPlaneTenants(1)...)
	s := New(&Config{}, live, testLogger())

	before, _, err := s.getAllPlaneClientCAs(planeTypeDataPlane, "prod")
	require.NoError(t, err)
	require.Equal(t, []byte("ca of tenant-0"), before["tenant-0/default"])

	secret := &corev1.Secret{}
	require.NoError(t, live.Get(context.Background(), client.ObjectKey{Namespace: "tenant-0", Name: "agent-ca"}, secret))
	secret.Data["ca.crt"] = []byte("rotated ca")
	require.NoError(t, live.Update(context.Background(), secret))

	notified, err := s.fetchCRClientCA(context.Background(), planeTypeDataPlane, "prod", "tenant-0", "default")
	require.NoError(t, err)
	require.Equal(t, []byte("rotated ca"), notified, "a notification must read the API server, not the cache")

	after, _, err := s.getAllPlaneClientCAs(planeTypeDataPlane, "prod")
	require.NoError(t, err)
	assert.Equal(t, []byte("rotated ca"), after["tenant-0/default"])
}

// When a CR's Secret can no longer be read, the CA that was read from it
// earlier must not go on authorizing new connections for the rest of its
// lifetime in the cache.
func TestConnect_DoesNotUseACachedCAAfterItsSecretFailsToRead(t *testing.T) {
	var calls apiCalls
	live := countingClient(&calls, sharedPlaneTenants(1)...)
	s := New(&Config{}, live, testLogger())

	before, _, err := s.getAllPlaneClientCAs(planeTypeDataPlane, "prod")
	require.NoError(t, err)
	require.Len(t, before, 1)

	require.NoError(t, live.Delete(context.Background(),
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "tenant-0", Name: "agent-ca"}}))
	_, err = s.fetchCRClientCA(context.Background(), planeTypeDataPlane, "prod", "tenant-0", "default")
	require.Error(t, err)

	after, _, err := s.getAllPlaneClientCAs(planeTypeDataPlane, "prod")
	require.NoError(t, err)
	assert.Empty(t, after)
}

// A plane CR is often created just before the Secret that holds its CA. The
// lookup then fails because the Secret does not exist yet, which is not the
// same as the CR being gone: the Secret can appear a moment later, and nothing
// prompts another attempt for a propagated event or a CR left unread by a
// connect. Only a missing CR ends the retries.
func TestRevalidation_RetriesWhileTheCASecretDoesNotExistYet(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, caCert, caKey)
	objects := sharedPlaneTenantsWithCA(1, encodeCertToPEM(t, caCert))

	var secretGets atomic.Int32
	live := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(objects...).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, isSecret := obj.(*corev1.Secret); isSecret && secretGets.Add(1) == 1 {
					return apierrors.NewNotFound(corev1.Resource("secrets"), key.Name)
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}).Build()
	s := New(&Config{}, live, testLogger())
	_, err := s.connMgr.Register("dataplane", "prod", &mockGatewayConn{}, []string{"other/default"}, clientCert, nil)
	require.NoError(t, err)

	s.ApplyPlaneEvent(fabric.PlaneEvent{
		PlaneType: "dataplane", PlaneID: "prod", Event: "updated",
		Namespace: "tenant-0", Name: "default",
	})

	_, err = s.connMgr.GetForCR("dataplane/prod", "tenant-0/default")
	require.NoError(t, err, "the CR must be authorized once its Secret exists")
	assert.Equal(t, int32(2), secretGets.Load())
}

// tenantCRKeys returns the CR keys of the first n tenants of sharedPlaneTenants.
func tenantCRKeys(n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = fmt.Sprintf("tenant-%d/default", i)
	}
	return keys
}

// A connect reaches its lookup bound when the API server is slow, and agents
// reconnect at such times too. If every partial connect ran its own job, the
// same CRs would be read once per connect, all at the same time, adding load
// exactly when the API server can least take it. One job per plane keeps the
// reads within the limit however many connects report unread CRs.
func TestUnreadCRs_OverlappingConnectsShareOneJob(t *testing.T) {
	const tenants = 60
	caCert, caKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, caCert, caKey)
	calls := apiCalls{secretDelay: 10 * time.Millisecond}
	s := New(&Config{}, countingClient(&calls, sharedPlaneTenantsWithCA(tenants, encodeCertToPEM(t, caCert))...), testLogger())
	_, err := s.connMgr.Register("dataplane", "prod", &mockGatewayConn{}, nil, clientCert, nil)
	require.NoError(t, err)

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		s.authorizeUnreadCRs("dataplane", "prod", tenantCRKeys(tenants))
	}()
	require.Eventually(t, func() bool { return calls.secretGets.Load() > 0 }, 5*time.Second, time.Millisecond)

	// A second connect reports the same CRs while the first job is running.
	// It must hand them over and return, not start reading alongside it.
	s.authorizeUnreadCRs("dataplane", "prod", tenantCRKeys(tenants))
	select {
	case <-firstDone:
		t.Fatal("the second call ran the work itself instead of handing it to the running job")
	default:
	}

	<-firstDone
	assert.LessOrEqual(t, calls.maxInFlight.Load(), int32(caReadConcurrency))
	for _, cr := range tenantCRKeys(tenants) {
		_, err := s.connMgr.GetForCR("dataplane/prod", cr)
		assert.NoError(t, err, "%s is not authorized", cr)
	}
}

// The job authorizes CRs on the plane's connections. Once the plane has no
// agent connected to this replica there is nothing to authorize, and an agent
// that connects later is checked against every CR again, so reading on would
// only load the API server.
func TestUnreadCRs_JobEndsWhenThePlaneHasNoAgent(t *testing.T) {
	const tenants = 200
	caCert, caKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, caCert, caKey)
	calls := apiCalls{secretDelay: 10 * time.Millisecond}
	s := New(&Config{}, countingClient(&calls, sharedPlaneTenantsWithCA(tenants, encodeCertToPEM(t, caCert))...), testLogger())
	connID, err := s.connMgr.Register("dataplane", "prod", &mockGatewayConn{}, nil, clientCert, nil)
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.authorizeUnreadCRs("dataplane", "prod", tenantCRKeys(tenants))
	}()
	require.Eventually(t, func() bool { return calls.secretGets.Load() > 0 }, 5*time.Second, time.Millisecond)

	s.connMgr.Unregister("dataplane/prod", connID)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the job kept running after the plane's last agent had gone")
	}
	assert.Less(t, calls.secretGets.Load(), int32(tenants), "every CR was read although no agent was left to authorize")

	// The plane is free for the job of the next connect.
	s.unreadMu.Lock()
	_, stillRegistered := s.unreadJobs["dataplane/prod"]
	s.unreadMu.Unlock()
	assert.False(t, stillRegistered)
}

// A retry waits between attempts. When the server shuts down, or the job a
// retry belongs to ends, the wait must end with it: an uninterruptible sleep
// holds a goroutine and a read slot for the whole backoff.
func TestRevalidation_BackoffEndsWithItsContext(t *testing.T) {
	previous := planeEventRetryBackoff
	planeEventRetryBackoff = time.Hour
	t.Cleanup(func() { planeEventRetryBackoff = previous })

	s := New(&Config{}, getFailingClient(liveDataPlane("ca")), testLogger())
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.revalidateCRWithRetry(ctx, updatedNs1Dp)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the retry outlived its context")
	}
}

// planeReaderOnly fails the test when it is asked for anything but a list. The
// gateway may watch plane CRs but not Secrets, so a Secret must never be read
// through the plane reader.
func planeReaderOnly(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()
	return fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(objects...).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(_ context.Context, _ client.WithWatch, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
				t.Errorf("%T %s was read through the plane reader", obj, key)
				return fmt.Errorf("not served by the plane reader")
			},
		}).Build()
}

// Listing every plane CR from the API server, on each agent connect and on
// each sweep, grows with the number of plane CRs in the cluster. Both lists
// must come from the plane reader, and the Secrets from the API server.
func TestPlaneCRLists_ComeFromThePlaneReader(t *testing.T) {
	const tenants = 5
	var planes, secrets []client.Object
	for _, obj := range sharedPlaneTenants(tenants) {
		if _, isSecret := obj.(*corev1.Secret); isSecret {
			secrets = append(secrets, obj)
		} else {
			planes = append(planes, obj)
		}
	}
	var calls apiCalls
	s := New(&Config{}, countingClient(&calls, secrets...), testLogger())
	s.SetPlaneReader(planeReaderOnly(t, planes...))

	cas, _, err := s.getAllPlaneClientCAs(planeTypeDataPlane, "prod")
	require.NoError(t, err)
	assert.Len(t, cas, tenants)

	existing, err := s.existingPlaneCRs(context.Background(), planeTypeDataPlane)
	require.NoError(t, err)
	assert.Len(t, existing, tenants)

	assert.Zero(t, calls.lists.Load(), "plane CRs were listed from the API server")
	assert.Equal(t, int32(tenants), calls.secretGets.Load())
}

// The plane reader can trail the API server. A CR that a notification
// authorized a moment ago may not be in it yet, and the sweep must not take
// that for a deletion and revoke the CR.
func TestSweep_KeepsACRThePlaneReaderHasNotSeenYet(t *testing.T) {
	var calls apiCalls
	s := New(&Config{}, countingClient(&calls, sharedPlaneTenants(1)...), testLogger())
	s.SetPlaneReader(planeReaderOnly(t)) // An informer cache that is still empty.

	_, err := s.connMgr.Register("dataplane", "prod", &mockGatewayConn{}, []string{"tenant-0/default"}, nil, nil)
	require.NoError(t, err)

	s.revokeDeletedCRs(context.Background())

	_, err = s.connMgr.GetForCR("dataplane/prod", "tenant-0/default")
	assert.NoError(t, err, "a CR that exists was revoked")
}

func TestSecretCACache(t *testing.T) {
	key := secretCAKey{namespace: "tenant-0", name: "agent-ca", key: "ca.crt"}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	newCache := func() (*secretCACache, *time.Time) {
		now := start
		c := newSecretCACache()
		c.now = func() time.Time { return now }
		return c, &now
	}

	// A CA changed only in its Secret reaches no notification. The bound on
	// how long a new connection can be checked against the old one is the TTL.
	t.Run("an entry is not used once it is older than the TTL", func(t *testing.T) {
		c, now := newCache()
		c.put(key, []byte("ca"), *now)

		*now = start.Add(caCacheTTL - time.Second)
		got, ok := c.get(key)
		require.True(t, ok)
		assert.Equal(t, []byte("ca"), got)

		*now = start.Add(caCacheTTL)
		_, ok = c.get(key)
		assert.False(t, ok)
	})

	// A connect and a notification can read one Secret at the same time and
	// finish in either order. The read that started later saw the newer
	// Secret, and must be the one that stays.
	t.Run("a read that started earlier does not replace a later one", func(t *testing.T) {
		c, now := newCache()
		c.put(key, []byte("new"), now.Add(2*time.Second))
		c.put(key, []byte("old"), now.Add(time.Second))

		*now = start.Add(3 * time.Second)
		got, ok := c.get(key)
		require.True(t, ok)
		assert.Equal(t, []byte("new"), got)
	})

	// A connect and a notification read one Secret at the same time. The
	// notification's read starts later and finds the Secret deleted. If the
	// connect's read, which still saw the old Secret, finishes after it, the
	// old CA must not come back: new connections would be checked against a
	// CA whose Secret no longer exists, for the rest of the TTL.
	t.Run("a read that started before a failed one does not restore the CA", func(t *testing.T) {
		c, now := newCache()
		c.put(key, []byte("old"), *now)

		c.drop(key, now.Add(2*time.Second))
		c.put(key, []byte("old"), now.Add(time.Second))

		*now = start.Add(3 * time.Second)
		_, ok := c.get(key)
		assert.False(t, ok)
	})

	// The other order: a failed read that started before a successful one says
	// nothing about the Secret as the later read found it.
	t.Run("a failed read does not remove a CA read after it started", func(t *testing.T) {
		c, now := newCache()
		c.put(key, []byte("new"), now.Add(2*time.Second))
		c.drop(key, now.Add(time.Second))

		*now = start.Add(3 * time.Second)
		got, ok := c.get(key)
		require.True(t, ok)
		assert.Equal(t, []byte("new"), got)
	})

	// Entries are only ever replaced by a read of the same Secret, so the
	// Secrets of deleted plane CRs would otherwise stay for the life of the pod.
	t.Run("expired entries are removed", func(t *testing.T) {
		c, now := newCache()
		c.put(key, []byte("ca"), *now)

		*now = start.Add(2 * caCacheTTL)
		other := secretCAKey{namespace: "tenant-1", name: "agent-ca", key: "ca.crt"}
		c.put(other, []byte("ca"), *now)

		assert.Len(t, c.entries, 1)
		assert.Contains(t, c.entries, other)
	})
}
