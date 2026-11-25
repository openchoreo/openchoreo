// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package renderedrelease

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// Each story is told once without trouble. Then it is told again with the controller stopped
// after every single write of its pass, with the answer to every single write lost, and with
// the status not written. It always has to end the same, after the passes that follow.
var _ = Describe("RenderedRelease after a crash", func() {
	// run is one telling of a story. The release renders into a namespace that existed
	// before, so the guard is on.
	type run struct {
		*releaseFixture
		namespace string
	}
	object := func(r run, apiVersion, kind, name, fields string) *unstructured.Unstructured {
		GinkgoHelper()
		obj := &unstructured.Unstructured{Object: map[string]any{}}
		Expect(json.Unmarshal([]byte(fields), &obj.Object)).To(Succeed())
		obj.SetAPIVersion(apiVersion)
		obj.SetKind(kind)
		obj.SetNamespace(r.namespace)
		obj.SetName(name)
		return obj
	}
	configMap := func(r run, name, value string) *unstructured.Unstructured {
		return object(r, "v1", "ConfigMap", name, `{"data": {"by": "`+value+`"}}`)
	}
	deployment := func(r run, image string) *unstructured.Unstructured {
		return object(r, "apps/v1", "Deployment", "api", `{"spec": {"replicas": 1, "selector": {"matchLabels": {"app": "api"}},
			"template": {"metadata": {"labels": {"app": "api"}}, "spec": {"containers": [{"name": "api", "image": "`+image+`"}]}}}}`)
	}
	outside := func(objs ...*unstructured.Unstructured) {
		GinkgoHelper()
		for _, obj := range objs {
			Expect(k8sClient.Create(ctx, obj, client.FieldOwner("helm"))).To(Succeed())
		}
	}
	// applied makes the release apply three objects: one it took over and claims, one it
	// claims and created, and one it never claimed.
	applied := func(r run) {
		GinkgoHelper()
		outside(configMap(r, "taken", "helm"))
		r.change(rendering(configMap(r, "taken", "release"), configMap(r, "created", "release"), configMap(r, "unclaimed", "release")),
			claiming(configMap(r, "taken", ""), configMap(r, "created", "")))
		r.mustPass()
		r.mustPass()
		Expect(r.live(configMap(r, "created", "")).GetLabels()).To(HaveKey(labels.LabelKeyClaimed))
	}

	stories := []struct {
		name string
		// tell brings the release to the pass that the trouble is in.
		tell func(r run)
		// then changes the release right after that pass.
		then    func(r run)
		objects func(r run) []*unstructured.Unstructured
	}{
		{
			name: "takes over two objects and creates one",
			tell: func(r run) {
				outside(deployment(r, "api:1"), configMap(r, "taken", "helm"))
				r.change(rendering(deployment(r, "api:2"), configMap(r, "taken", "release"), configMap(r, "created", "release")),
					claiming(deployment(r, ""), configMap(r, "taken", "")))
			},
			objects: func(r run) []*unstructured.Unstructured {
				return []*unstructured.Unstructured{deployment(r, ""), configMap(r, "taken", ""), configMap(r, "created", "")}
			},
		},
		{
			name: "creates an object it claims",
			tell: func(r run) {
				r.change(rendering(configMap(r, "created", "release"), configMap(r, "unclaimed", "release")), claiming(configMap(r, "created", "")))
			},
			objects: func(r run) []*unstructured.Unstructured {
				return []*unstructured.Unstructured{configMap(r, "created", ""), configMap(r, "unclaimed", "")}
			},
		},
		{
			name: "renders an object no more right after it created it",
			tell: func(r run) { r.change(rendering(configMap(r, "a", "release"), configMap(r, "dropped", "release"))) },
			then: func(r run) { r.change(rendering(configMap(r, "a", "release"))) },
			objects: func(r run) []*unstructured.Unstructured {
				return []*unstructured.Unstructured{configMap(r, "a", ""), configMap(r, "dropped", "")}
			},
		},
		{
			name: "moves to Observe right after it took an object over, and back",
			tell: func(r run) {
				outside(configMap(r, "taken", "release"))
				r.change(rendering(configMap(r, "taken", "release"), configMap(r, "created", "release")), claiming(configMap(r, "taken", "")))
			},
			then: func(r run) {
				r.change(inMode(openchoreov1alpha1.RenderedReleaseModeObserve))
				_, _ = r.pass()
				r.change(inMode(openchoreov1alpha1.RenderedReleaseModeManage))
			},
			objects: func(r run) []*unstructured.Unstructured {
				return []*unstructured.Unstructured{configMap(r, "taken", ""), configMap(r, "created", "")}
			},
		},
		{
			name: "moves to Observe with one claim left",
			tell: func(r run) {
				applied(r)
				r.change(inMode(openchoreov1alpha1.RenderedReleaseModeObserve), claiming(configMap(r, "taken", "")))
			},
			objects: func(r run) []*unstructured.Unstructured {
				return []*unstructured.Unstructured{configMap(r, "taken", ""), configMap(r, "created", ""), configMap(r, "unclaimed", "")}
			},
		},
		{
			name: "renders an object no more while it claims another for the first time",
			tell: func(r run) {
				r.change(rendering(configMap(r, "dropped", "release"), configMap(r, "late", "release")))
				r.mustPass()
				r.change(rendering(configMap(r, "late", "release")), claiming(configMap(r, "late", ""), configMap(r, "dropped", "")))
			},
			objects: func(r run) []*unstructured.Unstructured {
				return []*unstructured.Unstructured{configMap(r, "dropped", ""), configMap(r, "late", "")}
			},
		},
		{
			name: "is deleted",
			tell: func(r run) {
				applied(r)
				r.change(claiming(configMap(r, "taken", "")))
				r.deleteRelease()
			},
			objects: func(r run) []*unstructured.Unstructured {
				return []*unstructured.Unstructured{configMap(r, "taken", ""), configMap(r, "created", ""), configMap(r, "unclaimed", "")}
			},
		},
	}

	isStored := func(r *planeRequest) bool {
		return r.method != http.MethodGet && !strings.Contains(r.query, "dryRun=All")
	}
	// end describes how a telling ends: the objects with their marks, and the status.
	end := func(r run, objects []*unstructured.Unstructured, uid string) string {
		GinkgoHelper()
		var lines []string
		for _, obj := range objects {
			live := r.live(obj)
			if live == nil {
				lines = append(lines, describeObject(obj)+": gone")
				continue
			}
			managers := make([]string, 0, len(live.GetManagedFields()))
			for _, entry := range live.GetManagedFields() {
				managers = append(managers, fmt.Sprintf("%s/%s %s", entry.Manager, entry.Operation, entry.FieldsV1.GetRawString()))
			}
			slices.Sort(managers)
			line, err := json.Marshal(map[string]any{
				"generation": live.GetGeneration(), "labels": live.GetLabels(), "annotations": live.GetAnnotations(),
				"data": live.Object["data"], "spec": live.Object["spec"], "managers": managers,
			})
			Expect(err).NotTo(HaveOccurred())
			lines = append(lines, describeObject(obj)+": "+string(line))
		}
		release := &openchoreov1alpha1.RenderedRelease{}
		if err := k8sClient.Get(ctx, r.key(), release); apierrors.IsNotFound(err) {
			lines = append(lines, "release: gone")
		} else {
			Expect(err).NotTo(HaveOccurred())
			for _, resource := range release.Status.Resources {
				lines = append(lines, fmt.Sprintf("resource %s %s %s", resource.ID, resource.Kind, resource.HealthStatus))
			}
			for _, resource := range release.Status.ObservedResources {
				lines = append(lines, fmt.Sprintf("observed %s %s %s", resource.ID, resource.Kind, resource.HealthStatus))
			}
			for _, cond := range release.Status.Conditions {
				lines = append(lines, fmt.Sprintf("%s=%s/%s %s", cond.Type, cond.Status, cond.Reason, cond.Message))
			}
		}
		return strings.NewReplacer(r.namespace, "<namespace>", r.own, "<own>", r.name, "<release>", uid, "<uid>").Replace(strings.Join(lines, "\n"))
	}

	for i, story := range stories {
		It("ends the same when it "+story.name, func() {
			tellings := 0
			// tell tells the story with trouble in its pass, and returns how it ends and how
			// many writes the pass made.
			tell := func(trouble func(r run) context.Context) (string, int) {
				GinkgoHelper()
				tellings++
				name := fmt.Sprintf("crash-%d-%d", i, tellings)
				r := run{releaseFixture: newReleaseFixture(name, ""), namespace: name}
				Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})).To(Succeed())
				uid := string(fetchRelease(r.name).UID)
				story.tell(r)
				passCtx := trouble(r)
				_, _ = r.passIn(passCtx)
				writes := len(r.sentThat(isStored))

				// What follows is undisturbed. The first pass after it may still see the trouble,
				// such as an answer that was lost, and the last one must not.
				r.r.Client = k8sClient
				if story.then != nil {
					story.then(r)
				}
				for pass := range 4 {
					if apierrors.IsNotFound(k8sClient.Get(ctx, r.key(), &openchoreov1alpha1.RenderedRelease{})) {
						break
					}
					_, err := r.pass()
					if pass > 0 {
						Expect(err).NotTo(HaveOccurred())
					}
				}
				return end(r, story.objects(r), uid), writes
			}

			want, writes := tell(func(run) context.Context { return ctx })
			Expect(writes).To(BeNumerically(">=", 2), "there is something to disturb")

			kth := func(k int) func(*planeRequest) bool {
				seen := 0
				return func(r *planeRequest) bool {
					if isStored(r) {
						seen++
					}
					return isStored(r) && seen == k
				}
			}
			for k := 1; k <= writes; k++ {
				got, _ := tell(func(r run) context.Context {
					crashing, crash := context.WithCancel(ctx)
					r.proxy.hook(planeHook{match: kth(k), after: crash})
					return crashing
				})
				Expect(got).To(Equal(want), "stopped after write %d of %d", k, writes)

				got, _ = tell(func(r run) context.Context {
					r.proxy.hook(planeHook{match: kth(k), lose: true})
					return ctx
				})
				Expect(got).To(Equal(want), "the answer to write %d of %d was lost", k, writes)
			}

			got, _ := tell(func(r run) context.Context {
				base, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
				Expect(err).NotTo(HaveOccurred())
				r.r.Client = interceptor.NewClient(base, interceptor.Funcs{
					SubResourceUpdate: func(context.Context, client.Client, string, client.Object, ...client.SubResourceUpdateOption) error {
						return errors.New("the status can't be written")
					},
					Update: func(context.Context, client.WithWatch, client.Object, ...client.UpdateOption) error {
						return errors.New("the finalizer can't be removed")
					},
				})
				return ctx
			})
			Expect(got).To(Equal(want), "the status was not written")
		})
	}
})
