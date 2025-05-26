// Copyright OpenChoreo Authors 2025
// SPDX-License-Identifier: Apache-2.0

package v2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CiliumNetworkPolicy is a Kubernetes third-party resource with an extended
// version of NetworkPolicy.
type CiliumNetworkPolicy struct {
	// +deepequal-gen=false
	metav1.TypeMeta `json:",inline"`
	// +deepequal-gen=false
	metav1.ObjectMeta `json:"metadata"`

	// Spec is the desired Cilium specific rule specification.
	Spec *Rule `json:"spec,omitempty"`

	// Specs is a list of desired Cilium specific rule specification.
	Specs Rules `json:"specs,omitempty"`
}

// Rule is a policy rule which must be applied to all endpoints which match the
// labels contained in the endpointSelector
//
// Each rule is split into an ingress section which contains all rules
// applicable at ingress, and an egress section applicable at egress. For rule
// types such as `L4Rule` and `CIDR` which can be applied at both ingress and
// egress, both ingress and egress side have to either specifically allow the
// connection or one side has to be omitted.
//
// Either ingress, egress, or both can be provided. If both ingress and egress
// are omitted, the rule has no effect.
//
// +deepequal-gen:private-method=true
type Rule struct {
	// EndpointSelector selects all endpoints which should be subject to
	// this rule. EndpointSelector and NodeSelector cannot be both empty and
	// are mutually exclusive.
	//

	EndpointSelector *EndpointSelector `json:"endpointSelector,omitempty"`

	// NodeSelector selects all nodes which should be subject to this rule.
	// EndpointSelector and NodeSelector cannot be both empty and are mutually
	// exclusive. Can only be used in CiliumClusterwideNetworkPolicies.
	//

	NodeSelector *EndpointSelector `json:"nodeSelector,omitempty"`

	// Ingress is a list of IngressRule which are enforced at ingress.
	// If omitted or empty, this rule does not apply at ingress.
	//

	Ingress []IngressRule `json:"ingress,omitempty"`

	// Egress is a list of EgressRule which are enforced at egress.
	// If omitted or empty, this rule does not apply at egress.
	//

	Egress []EgressRule `json:"egress,omitempty"`

	// EgressDeny is a list of EgressDenyRule which are enforced at egress.
	// Any rule inserted here will be denied regardless of the allowed egress
	// rules in the 'egress' field.
	// If omitted or empty, this rule does not apply at egress.
	//

	EgressDeny []EgressDenyRule `json:"egressDeny,omitempty"`

	// Labels is a list of optional strings which can be used to
	// re-identify the rule or to store metadata. It is possible to lookup
	// or delete strings based on labels. Labels are not required to be
	// unique, multiple rules can have overlapping or identical labels.
	//

	Labels LabelArray `json:"labels,omitempty"`

	// Description is a free form string, it can be used by the creator of
	// the rule to store human readable explanation of the purpose of this
	// rule. Rules cannot be identified by comment.
	//

	Description string `json:"description,omitempty"`
}

// Rules is a collection of Rule.
//
// All rules must be evaluated in order to come to a conclusion. While
// it is sufficient to have a single fromEndpoints rule match, none of
// the fromRequires may be violated at the same time.
// +deepequal-gen:private-method=true
type Rules []*Rule

type IngressRule struct {
	// FromEndpoints is a list of endpoints identified by an
	// EndpointSelector which are allowed to communicate with the endpoint
	// subject to the rule.
	//
	// Example:
	// Any endpoint with the label "role=backend" can be consumed by any
	// endpoint carrying the label "role=frontend".
	FromEndpoints []EndpointSelector `json:"fromEndpoints,omitempty"`

	// FromRequires is a list of additional constraints which must be met
	// in order for the selected endpoints to be reachable. These
	// additional constraints do no by itself grant access privileges and
	// must always be accompanied with at least one matching FromEndpoints.
	//
	// Example:
	// Any Endpoint with the label "team=A" requires consuming endpoint
	// to also carry the label "team=A".
	FromRequires []EndpointSelector `json:"fromRequires,omitempty"`

	// FromCIDR is a list of IP blocks which the endpoint subject to the
	// rule is allowed to receive connections from. Only connections which
	// do *not* originate from the cluster or from the local host are subject
	// to CIDR rules. In order to allow in-cluster connectivity, use the
	// FromEndpoints field.  This will match on the source IP address of
	// incoming connections. Adding  a prefix into FromCIDR or into
	// FromCIDRSet with no ExcludeCIDRs is  equivalent.  Overlaps are
	// allowed between FromCIDR and FromCIDRSet.
	//
	// Example:
	// Any endpoint with the label "app=my-legacy-pet" is allowed to receive
	// connections from 10.3.9.1
	FromCIDR []CIDR `json:"fromCIDR,omitempty"`

	// FromCIDRSet is a list of IP blocks which the endpoint subject to the
	// rule is allowed to receive connections from in addition to FromEndpoints,
	// along with a list of subnets contained within their corresponding IP block
	// from which traffic should not be allowed.
	// This will match on the source IP address of incoming connections. Adding
	// a prefix into FromCIDR or into FromCIDRSet with no ExcludeCIDRs is
	// equivalent. Overlaps are allowed between FromCIDR and FromCIDRSet.
	//
	// Example:
	// Any endpoint with the label "app=my-legacy-pet" is allowed to receive
	// connections from 10.0.0.0/8 except from IPs in subnet 10.96.0.0/12.
	FromCIDRSet []CIDRRule `json:"fromCIDRSet,omitempty"`

	// ToPorts is a list of destination ports identified by port number and
	// protocol which the endpoint subject to the rule is allowed to
	// receive connections on.
	//
	// Example:
	// Any endpoint with the label "app=httpd" can only accept incoming
	// connections on port 80/tcp.
	ToPorts []PortRule `json:"toPorts,omitempty"`

	// FromEntities is a list of special entities which the endpoint subject
	// to the rule is allowed to receive connections from. Supported entities are
	// `world`, `cluster` and `host`
	//
	FromEntities []Entity `json:"fromEntities,omitempty"`
}

type EgressRule struct {
	// ToEndpoints is a list of endpoints identified by an EndpointSelector to
	// which the endpoints subject to the rule are allowed to communicate.
	//
	// Example:
	// Any endpoint with the label "role=frontend" can communicate with any
	// endpoint carrying the label "role=backend".
	ToEndpoints []EndpointSelector `json:"toEndpoints,omitempty"`

	// ToCIDR is a list of IP blocks which the endpoint subject to the rule
	// is allowed to initiate connections. Only connections destined for
	// outside of the cluster and not targeting the host will be subject
	// to CIDR rules.  This will match on the destination IP address of
	// outgoing connections. Adding a prefix into ToCIDR or into ToCIDRSet
	// with no ExcludeCIDRs is equivalent. Overlaps are allowed between
	// ToCIDR and ToCIDRSet.
	//
	// Example:
	// Any endpoint with the label "app=database-proxy" is allowed to
	// initiate connections to 10.2.3.0/24
	ToCIDR []CIDR `json:"toCIDR,omitempty"`

	// ToCIDRSet is a list of IP blocks which the endpoint subject to the rule
	// is allowed to initiate connections to in addition to connections
	// which are allowed via ToEndpoints, along with a list of subnets contained
	// within their corresponding IP block to which traffic should not be
	// allowed. This will match on the destination IP address of outgoing
	// connections. Adding a prefix into ToCIDR or into ToCIDRSet with no
	// ExcludeCIDRs is equivalent. Overlaps are allowed between ToCIDR and
	// ToCIDRSet.
	//
	// Example:
	// Any endpoint with the label "app=database-proxy" is allowed to
	// initiate connections to 10.2.3.0/24 except from IPs in subnet 10.2.3.0/28.
	ToCIDRSet []CIDRRule `json:"toCIDRSet,omitempty"`

	// ToPorts is a list of destination ports identified by port number and
	// protocol which the endpoint subject to the rule is allowed to
	// connect to.
	//
	// Example:
	// Any endpoint with the label "role=frontend" is allowed to initiate
	// connections to destination port 8080/tcp
	ToPorts []PortRule `json:"toPorts,omitempty"`

	// ToFQDN allows whitelisting DNS names in place of IPs. The IPs that result
	// from DNS resolution of `ToFQDN.MatchName`s are added to the same
	// EgressRule object as ToCIDRSet entries, and behave accordingly. Any L4 and
	// L7 rules within this EgressRule will also apply to these IPs.
	// The DNS -> IP mapping is re-resolved periodically from within the
	// cilium-agent, and the IPs in the DNS response are effected in the policy
	// for selected pods as-is (i.e. the list of IPs is not modified in any way).
	// Note: An explicit rule to allow for DNS traffic is needed for the pods, as
	// ToFQDN counts as an egress rule and will enforce egress policy when
	// PolicyEnforcment=default.
	// Note: If the resolved IPs are IPs within the kubernetes cluster, the
	// ToFQDN rule will not apply to that IP.
	// Note: ToFQDN cannot occur in the same policy as other To* rules.
	//
	ToFQDNs []FQDNSelector `json:"toFQDNs,omitempty"`
}

type EgressDenyRule struct {
	// ToCIDR is a list of IP blocks which the endpoint subject to the rule
	// is allowed to initiate connections. Only connections destined for
	// outside of the cluster and not targeting the host will be subject
	// to CIDR rules.  This will match on the destination IP address of
	// outgoing connections. Adding a prefix into ToCIDR or into ToCIDRSet
	// with no ExcludeCIDRs is equivalent. Overlaps are allowed between
	// ToCIDR and ToCIDRSet.
	//
	// Example:
	// Any endpoint with the label "app=database-proxy" is allowed to
	// initiate connections to 10.2.3.0/24
	//
	ToCIDR []CIDR `json:"toCIDR,omitempty"`

	// ToCIDRSet is a list of IP blocks which the endpoint subject to the rule
	// is allowed to initiate connections to in addition to connections
	// which are allowed via ToEndpoints, along with a list of subnets contained
	// within their corresponding IP block to which traffic should not be
	// allowed. This will match on the destination IP address of outgoing
	// connections. Adding a prefix into ToCIDR or into ToCIDRSet with no
	// ExcludeCIDRs is equivalent. Overlaps are allowed between ToCIDR and
	// ToCIDRSet.
	//
	// Example:
	// Any endpoint with the label "app=database-proxy" is allowed to
	// initiate connections to 10.2.3.0/24 except from IPs in subnet 10.2.3.0/28.
	ToCIDRSet []CIDRRule `json:"toCIDRSet,omitempty"`
}

// Entity specifies the class of receiver/sender endpoints that do not have
// individual identities.  Entities are used to describe "outside of cluster",
// "host", etc.
type Entity string

const (
	// EntityAll is an entity that represents all traffic
	EntityAll Entity = "all"

	// EntityWorld is an entity that represents traffic external to
	// endpoint's cluster
	EntityWorld Entity = "world"

	// EntityCluster is an entity that represents traffic within the
	// endpoint's cluster, to endpoints not managed by cilium
	EntityCluster Entity = "cluster"

	// EntityHost is an entity that represents traffic within endpoint host
	EntityHost Entity = "host"

	// EntityInit is an entity that represents an initializing endpoint
	EntityInit Entity = "init"

	// EntityIngress is an entity that represents envoy proxy
	EntityIngress Entity = "ingress"

	// EntityUnmanaged is an entity that represents unamanaged endpoints.
	EntityUnmanaged Entity = "unmanaged"

	// EntityRemoteNode is an entity that represents all remote nodes
	EntityRemoteNode Entity = "remote-node"

	// EntityHealth is an entity that represents all health endpoints.
	EntityHealth Entity = "health"

	// EntityNone is an entity that can be selected but never exist
	EntityNone Entity = "none"

	// EntityKubeAPIServer is an entity that represents the kube-apiserver.
	EntityKubeAPIServer Entity = "kube-apiserver"
)

// CIDR specifies a block of IP addresses.
// Example: 192.0.2.1/32
type CIDR = string

// CIDRRule is a rule that specifies a CIDR prefix to/from which outside
// communication  is allowed, along with an optional list of subnets within that
// CIDR prefix to/from which outside communication is not allowed.
type CIDRRule struct {
	// CIDR is a CIDR prefix / IP Block.
	Cidr CIDR `json:"cidr"`

	// ExceptCIDRs is a list of IP blocks which the endpoint subject to the rule
	// is not allowed to initiate connections to. These CIDR prefixes should be
	// contained within Cidr. These exceptions are only applied to the Cidr in
	// this CIDRRule, and do not apply to any other CIDR prefixes in any other
	// CIDRRules.
	ExceptCIDRs []CIDR `json:"except,omitempty"`
}

type FQDNSelector struct {
	// MatchName matches literal DNS names. A trailing "." is automatically added
	// when missing.
	//

	MatchName string `json:"matchName,omitempty"`

	// MatchPattern allows using wildcards to match DNS names. All wildcards are
	// case insensitive. The wildcards are:
	// - "*" matches 0 or more DNS valid characters, and may occur anywhere in
	// the pattern. As a special case a "*" as the leftmost character, without a
	// following "." matches all subdomains as well as the name to the right.
	// A trailing "." is automatically added when missing.
	//
	// Examples:
	// `*.cilium.io` matches subomains of cilium at that level
	//   www.cilium.io and blog.cilium.io match, cilium.io and google.com do not
	// `*cilium.io` matches cilium.io and all subdomains ends with "cilium.io"
	//   except those containing "." separator, subcilium.io and sub-cilium.io match,
	//   www.cilium.io and blog.cilium.io does not
	// sub*.cilium.io matches subdomains of cilium where the subdomain component
	// begins with "sub"
	//   sub.cilium.io and subdomain.cilium.io match, www.cilium.io,
	//   blog.cilium.io, cilium.io and google.com do not
	//

	MatchPattern string `json:"matchPattern,omitempty"`
}

func CIDRMatchAllExcept(exceptCIDRs []CIDR) CIDRRule {
	return CIDRRule{
		Cidr:        "0.0.0.0/0",
		ExceptCIDRs: exceptCIDRs,
	}
}

// L4Proto is a layer 4 protocol name
type L4Proto string

const (
	ProtoTCP L4Proto = "TCP"
	ProtoUDP L4Proto = "UDP"
	ProtoAny L4Proto = "ANY"

	PortProtocolAny = "0/ANY"
)

// PortProtocol specifies an L4 port with an optional transport protocol
type PortProtocol struct {
	// Port is an L4 port number. For now the string will be strictly
	// parsed as a single uint16. In the future, this field may support
	// ranges in the form "1024-2048
	// Port can also be a port name, which must contain at least one [a-z],
	// and may also contain [0-9] and '-' anywhere except adjacent to another
	// '-' or in the beginning or the end.
	Port string `json:"port"`

	// Protocol is the L4 protocol. If omitted or empty, any protocol
	// matches. Accepted values: "TCP", "UDP", "SCTP", "ANY"
	//
	// Matching on ICMP is not supported.
	//
	// Named port specified for a container may narrow this down, but may not
	// contradict this.
	Protocol L4Proto `json:"protocol,omitempty"`
}

// PortRule is a list of ports/protocol combinations with optional Layer 7
// rules which must be met.
type PortRule struct {
	// Ports is a list of L4 port/protocol
	Ports []PortProtocol `json:"ports,omitempty"`
}

// PortDenyRule is a list of ports/protocol that should be used for deny
// policies. This structure lacks the L7Rules since it's not supported in deny
// policies.
type PortDenyRule struct {
	// Ports is a list of L4 port/protocol
	Ports []PortProtocol `json:"ports,omitempty"`
}

type EndpointSelector struct {
	MatchLabels      map[string]string `json:"matchLabels,omitempty"`
	MatchExpressions []MatchExpression `json:"matchExpressions,omitempty"`
}

type MatchExpression struct {
	Key      string   `json:"key"`
	Operator string   `json:"operator"`
	Values   []string `json:"values,omitempty"`
}

type LabelArray []Label

type Label struct {
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

// CiliumNetworkPolicyList is a list of CiliumNetworkPolicy objects.
type CiliumNetworkPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	// Items is a list of CiliumNetworkPolicy
	Items []CiliumNetworkPolicy `json:"items"`
}
