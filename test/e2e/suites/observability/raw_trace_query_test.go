// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive

	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

// Raw trace queries take the tracing backend's native request and return its
// native response. They are not scoped, so they need the cluster-scoped
// traces:rawquery permission. The suite installs the OpenSearch tracing module,
// so the allowed request is an OpenSearch search body.
var _ = Describe("Raw trace queries", Ordered, Label("tier3"), func() {
	SetDefaultEventuallyTimeout(framework.DefaultTimeout)
	SetDefaultEventuallyPollingInterval(framework.DefaultPolling)

	var adminToken, subjectToken string

	// lastHour is an OpenSearch search body counting the traces of the last hour.
	lastHour := func() string {
		end := time.Now().UTC()
		return fmt.Sprintf(`{"size":0,`+
			`"query":{"range":{"startTime":{"gte":%q,"lte":%q}}},`+
			`"aggs":{"traces":{"cardinality":{"field":"traceId"}}}}`,
			end.Add(-time.Hour).Format(time.RFC3339), end.Format(time.RFC3339))
	}

	BeforeAll(func() {
		ensureObservabilityFixtures()

		By("minting an admin token (service_mcp_client, granted admin)")
		Eventually(func() error {
			var err error
			adminToken, err = framework.AcquireObserverToken(observerQ)
			return err
		}, 60*time.Second, 5*time.Second).Should(Succeed())

		By("minting a subject token (mcp-e2e-subject-client, bound to no role)")
		Eventually(func() error {
			var err error
			subjectToken, err = framework.AcquireObserverTokenForClient(observerQ, obsSubjectClientID, obsSubjectClientSecret)
			return err
		}, 60*time.Second, 5*time.Second).Should(Succeed())
	})

	It("rejects an empty request", func() {
		status, body, err := framework.QueryRawTraces(observerQ, adminToken, "text/plain", " ")
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal(http.StatusBadRequest), body)
		Expect(body).To(ContainSubstring("OBS-V1-T-06"))
	})

	It("rejects a media type other than JSON or text", func() {
		status, body, err := framework.QueryRawTraces(observerQ, adminToken,
			"application/x-www-form-urlencoded", "query=match_all")
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal(http.StatusUnsupportedMediaType), body)
		Expect(body).To(ContainSubstring("OBS-V1-T-14"))
	})

	It("denies a caller without traces:rawquery", func() {
		status, body, err := framework.QueryRawTraces(observerQ, subjectToken, "application/json", lastHour())
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal(http.StatusForbidden), body)
	})

	It("runs an OpenSearch search body and returns OpenSearch's response", func() {
		status, body, err := framework.QueryRawTraces(observerQ, adminToken, "application/json", lastHour())
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal(http.StatusOK), body)

		// The e2e workloads send no spans, so there may be no trace index yet.
		// OpenSearch then searches no shards and leaves out "aggregations", so
		// the check is that the answer is OpenSearch's own search response.
		var resp struct {
			Shards       map[string]any `json:"_shards"`
			Hits         map[string]any `json:"hits"`
			Aggregations map[string]any `json:"aggregations"`
		}
		Expect(json.Unmarshal([]byte(body), &resp)).To(Succeed(), body)
		Expect(resp.Shards).To(HaveKey("total"), "the response should be OpenSearch's own: %s", body)
		Expect(resp.Hits).To(HaveKey("hits"), "the response should be OpenSearch's own: %s", body)
		if resp.Shards["total"] != float64(0) {
			Expect(resp.Aggregations).To(HaveKey("traces"), "a search of the trace indices should answer the aggregation: %s", body)
		}
	})
})
