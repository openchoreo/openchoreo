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
	// tokens, when set, holds every Secret read until it has taken a token or
	// the channel is closed, whatever its context does. A test uses it to
	// decide when a read finishes.
	tokens chan struct{}
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
					if calls.tokens != nil {
						<-calls.tokens
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
	calls := apiCalls{tokens: make(chan struct{})}
	s := New(&Config{}, countingClient(&calls, sharedPlaneTenantsWithCA(tenants, encodeCertToPEM(t, caCert))...), testLogger())
	_, err := s.connMgr.Register("dataplane", "prod", &mockGatewayConn{}, nil, clientCert, nil)
	require.NoError(t, err)

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		s.authorizeUnreadCRs("dataplane", "prod", tenantCRKeys(tenants))
	}()
	// The first job is reading, and stays there until the test lets it go.
	require.Eventually(t, func() bool { return calls.inFlight.Load() == caReadConcurrency }, 5*time.Second, time.Millisecond)

	// A second connect reports the same CRs. It must hand them to the running
	// job and return, not start reading alongside it.
	secondReturned := make(chan struct{})
	go func() {
		defer close(secondReturned)
		s.authorizeUnreadCRs("dataplane", "prod", tenantCRKeys(tenants))
	}()
	select {
	case <-secondReturned:
	case <-time.After(5 * time.Second):
		t.Fatal("the second call ran the work itself instead of handing it to the running job")
	}

	close(calls.tokens)
	<-firstDone
	assert.LessOrEqual(t, calls.maxInFlight.Load(), int32(caReadConcurrency))
	for _, cr := range tenantCRKeys(tenants) {
		_, err := s.connMgr.GetForCR("dataplane/prod", cr)
		assert.NoError(t, err, "%s is not authorized", cr)
	}
}

// When the plane's last agent goes, the job ends, but its workers take a moment
// to wind down. An agent can connect in that moment. If its unread CRs were
// added to the ending job they would be dropped with the rest, and the new
// agent would stay without them. The ending job must already be out of the
// way, so that those CRs start a job of their own.
func TestUnreadCRs_ANewAgentIsNotHandedToAJobThatIsEnding(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	clientCert := generateTestClientCert(t, caCert, caKey)
	calls := apiCalls{tokens: make(chan struct{})}
	s := New(&Config{}, countingClient(&calls, sharedPlaneTenantsWithCA(3, encodeCertToPEM(t, caCert))...), testLogger())
	firstAgent, err := s.connMgr.Register("dataplane", "prod", &mockGatewayConn{}, nil, clientCert, nil)
	require.NoError(t, err)

	endingJob := make(chan struct{})
	go func() {
		defer close(endingJob)
		s.authorizeUnreadCRs("dataplane", "prod", []string{"tenant-0/default", "tenant-1/default"})
	}()
	require.Eventually(t, func() bool { return calls.inFlight.Load() == 2 }, 5*time.Second, time.Millisecond)

	// The last agent goes. One read finishes, its worker sees that and ends
	// the job, while the other worker is still reading.
	s.connMgr.Unregister("dataplane/prod", firstAgent)
	calls.tokens <- struct{}{}
	require.Eventually(t, func() bool {
		s.unreadMu.Lock()
		defer s.unreadMu.Unlock()
		_, registered := s.unreadJobs["dataplane/prod"]
		return !registered
	}, 5*time.Second, time.Millisecond, "the ending job still takes new CRs")

	_, err = s.connMgr.Register("dataplane", "prod", &mockGatewayConn{}, nil, clientCert, nil)
	require.NoError(t, err)
	newJob := make(chan struct{})
	go func() {
		defer close(newJob)
		s.authorizeUnreadCRs("dataplane", "prod", []string{"tenant-2/default"})
	}()

	close(calls.tokens)
	<-endingJob
	<-newJob
	_, err = s.connMgr.GetForCR("dataplane/prod", "tenant-2/default")
	assert.NoError(t, err, "the new agent's unread CR was dropped")
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

// planeOf returns the DataPlane namespace/dp on plane "prod" at the given
// generation, with caPEM as its inline client CA.
func planeOf(namespace string, generation int64, caPEM []byte) *openchoreov1alpha1.DataPlane {
	return &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "dp", Namespace: namespace, Generation: generation},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "prod",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{Value: string(caPEM)},
			},
		},
	}
}

// crGetCountingClient serves objects and counts reads of a DataPlane.
func crGetCountingClient(crGets *atomic.Int32, objects ...client.Object) client.Client {
	return fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(objects...).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, isPlane := obj.(*openchoreov1alpha1.DataPlane); isPlane {
					crGets.Add(1)
				}
				return c.Get(ctx, key, obj, opts...)
			},
		}).Build()
}

// The plane reader is an informer cache and can trail the API server. Take a
// CR whose client CA was just rotated. The notification read the new CA from
// the API server and refused the agents that hold a certificate of the old
// one. If such an agent reconnects before the cache has seen the rotation, a
// connect that trusted the cached copy would authorize it again from the old
// CA, and no later event would take that back.
func TestConnect_DoesNotTrustAPlaneReaderOlderThanANotification(t *testing.T) {
	oldCA, oldKey := generateTestCA(t)
	newCA, _ := generateTestCA(t)
	agentCert := generateTestClientCert(t, oldCA, oldKey)
	oldPEM, newPEM := encodeCertToPEM(t, oldCA), encodeCertToPEM(t, newCA)

	// ns2/dp is untouched and still trusts the agent, so the connect goes ahead.
	other := planeOf("ns2", 1, oldPEM)
	live := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(planeOf("ns1", 2, newPEM), other).Build()
	s := New(&Config{}, live, testLogger())
	s.SetPlaneReader(planeReaderOnly(t, planeOf("ns1", 1, oldPEM), other.DeepCopy()))

	// The notification for the rotation reads the CR from the API server.
	_, err := s.fetchCRClientCA(context.Background(), planeTypeDataPlane, "prod", "ns1", "dp")
	require.NoError(t, err)

	validCRs, unread, err := s.verifyClientCertificatePerCR(agentCert, nil, planeTypeDataPlane, "prod")
	require.NoError(t, err)
	assert.Equal(t, []string{"ns2/dp"}, validCRs, "the rotated CR was granted from the cached copy")
	assert.Empty(t, unread, "the CR was read from the API server, so nothing is left for later")
}

// Once the plane reader has the version the notification read, its copy is as
// good as the API server's and the connect uses it again. Reading the CR on
// every connect from then on would bring back a read per notified CR.
func TestConnect_TrustsThePlaneReaderOnceItHasCaughtUp(t *testing.T) {
	ca, key := generateTestCA(t)
	agentCert := generateTestClientCert(t, ca, key)
	plane := planeOf("ns1", 2, encodeCertToPEM(t, ca))
	var crGets atomic.Int32
	s := New(&Config{}, crGetCountingClient(&crGets, plane), testLogger())
	s.SetPlaneReader(planeReaderOnly(t, plane.DeepCopy()))

	_, err := s.fetchCRClientCA(context.Background(), planeTypeDataPlane, "prod", "ns1", "dp")
	require.NoError(t, err)
	require.Equal(t, int32(1), crGets.Load())

	validCRs, unread, err := s.verifyClientCertificatePerCR(agentCert, nil, planeTypeDataPlane, "prod")
	require.NoError(t, err)
	assert.Equal(t, []string{"ns1/dp"}, validCRs)
	assert.Empty(t, unread)
	assert.Equal(t, int32(1), crGets.Load(), "the connect read a CR the plane reader already had")
}

// A CR that moves to this plane is notified here, and the notification
// authorizes it on the agents already connected. An agent that connects before
// the cache has seen the move finds the CR still listed under its old planeID.
// Passing over it for that reason would leave the new connection without the
// CR, and nothing would deliver it later.
func TestConnect_FindsACRThatMovedHereBeforeThePlaneReaderSawIt(t *testing.T) {
	ca, key := generateTestCA(t)
	agentCert := generateTestClientCert(t, ca, key)
	pem := encodeCertToPEM(t, ca)

	cached := planeOf("ns1", 1, pem)
	cached.Spec.PlaneID = "elsewhere"
	live := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(planeOf("ns1", 2, pem)).Build()
	s := New(&Config{}, live, testLogger())
	s.SetPlaneReader(planeReaderOnly(t, cached))

	_, err := s.fetchCRClientCA(context.Background(), planeTypeDataPlane, "prod", "ns1", "dp")
	require.NoError(t, err)

	validCRs, unread, err := s.verifyClientCertificatePerCR(agentCert, nil, planeTypeDataPlane, "prod")
	require.NoError(t, err)
	assert.Equal(t, []string{"ns1/dp"}, validCRs)
	assert.Empty(t, unread)
}

// The same holds for a CR that was created and notified a moment ago and that
// the plane reader does not list at all yet.
func TestConnect_FindsACRThePlaneReaderDoesNotListYet(t *testing.T) {
	ca, key := generateTestCA(t)
	agentCert := generateTestClientCert(t, ca, key)
	pem := encodeCertToPEM(t, ca)

	live := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(planeOf("ns1", 1, pem)).Build()
	s := New(&Config{}, live, testLogger())
	s.SetPlaneReader(planeReaderOnly(t))

	_, err := s.fetchCRClientCA(context.Background(), planeTypeDataPlane, "prod", "ns1", "dp")
	require.NoError(t, err)

	validCRs, _, err := s.verifyClientCertificatePerCR(agentCert, nil, planeTypeDataPlane, "prod")
	require.NoError(t, err)
	assert.Equal(t, []string{"ns1/dp"}, validCRs)
}

// And for a CR that is gone: once a read has found it deleted, a plane reader
// that still lists it must not get it authorized again.
func TestConnect_DoesNotAuthorizeADeletedCRThePlaneReaderStillLists(t *testing.T) {
	ca, key := generateTestCA(t)
	agentCert := generateTestClientCert(t, ca, key)
	pem := encodeCertToPEM(t, ca)

	other := planeOf("ns2", 1, pem)
	live := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(other).Build()
	s := New(&Config{}, live, testLogger())
	s.SetPlaneReader(planeReaderOnly(t, planeOf("ns1", 1, pem), other.DeepCopy()))

	_, err := s.fetchCRClientCA(context.Background(), planeTypeDataPlane, "prod", "ns1", "dp")
	require.Error(t, err)

	validCRs, unread, err := s.verifyClientCertificatePerCR(agentCert, nil, planeTypeDataPlane, "prod")
	require.NoError(t, err)
	assert.Equal(t, []string{"ns2/dp"}, validCRs)
	assert.Empty(t, unread)
}

// A CR can be deleted and created again under the same name. What this replica
// remembers about the first one says nothing about the second, whose
// generation starts over. Comparing the two would make the new CR look out of
// date on every connect, and if it is the plane's only CR every connect would
// be refused. The record must give way to what the API server holds now.
func TestConnect_IsNotHeldUpByTheRecordOfAnEarlierCROfTheSameName(t *testing.T) {
	ca, key := generateTestCA(t)
	agentCert := generateTestClientCert(t, ca, key)
	pem := encodeCertToPEM(t, ca)

	earlier := planeOf("ns1", 5, pem)
	earlier.UID = "earlier"
	replacement := planeOf("ns1", 1, pem)
	replacement.UID = "replacement"

	var crGets atomic.Int32
	s := New(&Config{}, crGetCountingClient(&crGets, replacement), testLogger())
	s.SetPlaneReader(planeReaderOnly(t, replacement.DeepCopy()))
	s.noteLiveCR(planeTypeDataPlane, "ns1/dp", earlier, time.Now())

	validCRs, unread, err := s.verifyClientCertificatePerCR(agentCert, nil, planeTypeDataPlane, "prod")
	require.NoError(t, err, "the connect was refused over a CR that no longer exists")
	assert.Equal(t, []string{"ns1/dp"}, validCRs)
	assert.Empty(t, unread)

	// The record now describes the replacement, which the plane reader has.
	reads := crGets.Load()
	_, _, err = s.verifyClientCertificatePerCR(agentCert, nil, planeTypeDataPlane, "prod")
	require.NoError(t, err)
	assert.Equal(t, reads, crGets.Load(), "the CR is read again on every connect")
}

// Two reads of one CR can finish in either order. Here a read that began before
// a CA rotation, and still saw the old CA, records its result after the
// notification for the rotation has recorded the new one. If it replaced that
// record, the plane reader's copy, which is just as old, would pass for
// current, and a connect would authorize an agent from the CA that was rotated
// out.
func TestConnect_AnOlderReadDoesNotReplaceWhatANewerOneRecorded(t *testing.T) {
	oldCA, oldKey := generateTestCA(t)
	newCA, _ := generateTestCA(t)
	agentCert := generateTestClientCert(t, oldCA, oldKey)
	oldPEM, newPEM := encodeCertToPEM(t, oldCA), encodeCertToPEM(t, newCA)

	other := planeOf("ns2", 1, oldPEM)
	live := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(planeOf("ns1", 2, newPEM), other).Build()
	s := New(&Config{}, live, testLogger())
	s.SetPlaneReader(planeReaderOnly(t, planeOf("ns1", 1, oldPEM), other.DeepCopy()))

	olderReadBegan := time.Now()
	_, err := s.fetchCRClientCA(context.Background(), planeTypeDataPlane, "prod", "ns1", "dp")
	require.NoError(t, err)
	s.noteLiveCR(planeTypeDataPlane, "ns1/dp", planeOf("ns1", 1, oldPEM), olderReadBegan)

	validCRs, _, err := s.verifyClientCertificatePerCR(agentCert, nil, planeTypeDataPlane, "prod")
	require.NoError(t, err)
	assert.Equal(t, []string{"ns2/dp"}, validCRs, "the rotated CR was granted from the cached copy")
}

// A notification applies a CR to the connections that are registered. A connect
// that is under way is not registered yet: it is authorized from what its
// lookup read before the change, and the notification passes it by. After a CA
// rotation that leaves the new connection holding a grant the rotation was
// meant to end. Once registered, the connect has to apply what was read while
// it was under way.
func TestConnect_AppliesANotificationThatArrivedWhileItWasUnderWay(t *testing.T) {
	oldCA, oldKey := generateTestCA(t)
	newCA, _ := generateTestCA(t)
	agentCert := generateTestClientCert(t, oldCA, oldKey)

	live := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(planeOf("ns1", 1, encodeCertToPEM(t, oldCA))).Build()
	s := New(&Config{}, live, testLogger())
	s.SetPlaneReader(planeReaderOnly(t, planeOf("ns1", 1, encodeCertToPEM(t, oldCA))))

	lookupStart := time.Now()
	validCRs, unread, err := s.verifyClientCertificatePerCR(agentCert, nil, planeTypeDataPlane, "prod")
	require.NoError(t, err)
	require.Equal(t, []string{"ns1/dp"}, validCRs)

	// The CA is rotated and the notification is handled before the connection
	// is registered.
	rotated := &openchoreov1alpha1.DataPlane{}
	require.NoError(t, live.Get(context.Background(), client.ObjectKey{Namespace: "ns1", Name: "dp"}, rotated))
	rotated.Spec.ClusterAgent.ClientCA.Value = string(encodeCertToPEM(t, newCA))
	require.NoError(t, live.Update(context.Background(), rotated))
	_, err = s.fetchCRClientCA(context.Background(), planeTypeDataPlane, "prod", "ns1", "dp")
	require.NoError(t, err)

	_, err = s.connMgr.Register("dataplane", "prod", &mockGatewayConn{}, validCRs, agentCert, nil)
	require.NoError(t, err)
	s.authorizeAfterConnect("dataplane", "prod", unread, lookupStart)

	_, err = s.connMgr.GetForCR("dataplane/prod", "ns1/dp")
	assert.Error(t, err, "the connection kept a grant from the CA that was rotated out")
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
