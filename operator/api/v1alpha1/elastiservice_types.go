/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ElastiServiceFinalizer = "elasti.truefoundry.com/finalizer"
)

// EnabledPeriod defines when the scale-to-zero policy is active.
// Outside of this period, services maintain minTargetReplicas and scale-down is prevented.
type EnabledPeriod struct {
	// Schedule is a 5-item cron expression (minute hour day month weekday).
	// Uses UTC timezone. Example: "0 9 * * 1-5" for 9 AM Monday-Friday.
	// +kubebuilder:default="0 0 * * *"
	Schedule string `json:"schedule,omitempty"`

	// Duration specifies how long the enabled period lasts from each scheduled trigger.
	// Accepts formats like "1h", "30m", "8h", etc.
	// +kubebuilder:default="24h"
	Duration string `json:"duration,omitempty"`
}

// +kubebuilder:validation:Required={"scaleTargetRef","service"}
type ElastiServiceSpec struct {
	// ScaleTargetRef of the target resource to scale
	ScaleTargetRef ScaleTargetRef `json:"scaleTargetRef"`
	// Service to scale
	Service string `json:"service"`
	// ProbeResponse defines synthetic HTTP responses the resolver serves locally (no proxy) while
	// scaled to zero, e.g. for load balancer health checks. First matching rule wins.
	// Each rule uses the same match fields and semantics as Gateway API HTTPRouteMatch
	// (https://gateway-api.sigs.k8s.io/reference/1.5/spec/#httproutematch): path, headers,
	// queryParams, and method are ANDed; omitted path defaults to prefix "/".
	// +optional
	// +kubebuilder:validation:MaxItems=32
	ProbeResponse []ProbeResponseRule `json:"probeResponse,omitempty"`
	// Minimum number of replicas to scale to
	// +kubebuilder:validation:Minimum=1
	MinTargetReplicas int32 `json:"minTargetReplicas,omitempty" default:"1"`
	// Cooldown period in seconds.
	// It tells how long a target resource can be idle before scaling it down
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=604800
	// +kubebuilder:default=900
	CooldownPeriod int32 `json:"cooldownPeriod,omitempty"`
	// Triggers to scale the target resource
	// +kubebuilder:validation:MinItems=1
	Triggers   []ScaleTrigger  `json:"triggers,omitempty"`
	Autoscaler *AutoscalerSpec `json:"autoscaler,omitempty"`
	// EnabledPeriod defines when the scale-to-zero policy is active.
	// When omitted, scale-to-zero is always enabled (default behavior).
	// When specified, scale-down only occurs during the cron schedule window.
	EnabledPeriod *EnabledPeriod `json:"enabledPeriod,omitempty"`
}

// ProbeResponseRule is one local response when an incoming request matches path, headers,
// queryParams, and method with the same semantics as Gateway API HTTPRouteMatch
// (https://gateway-api.sigs.k8s.io/reference/spec/#gateway.networking.k8s.io/v1.HTTPRouteMatch).
type ProbeResponseRule struct {
	// Path specifies a HTTP request path matcher. Omitted defaults to prefix "/" in the resolver.
	// +optional
	Path *ProbeResponsePathMatch `json:"path,omitempty"`
	// Headers specifies HTTP request header matchers (ANDed).
	// +optional
	// +kubebuilder:validation:MaxItems=16
	Headers []ProbeResponseHeaderMatch `json:"headers,omitempty"`
	// QueryParams specifies HTTP query parameter matchers (ANDed).
	// +optional
	// +kubebuilder:validation:MaxItems=16
	QueryParams []ProbeResponseQueryParamMatch `json:"queryParams,omitempty"`
	// Method, when set, matches the HTTP method.
	// +optional
	// +kubebuilder:validation:Enum=GET;HEAD;POST;PUT;DELETE;CONNECT;OPTIONS;TRACE;PATCH
	Method *string `json:"method,omitempty"`
	// Response is the literal response body Elasti returns with HTTP 200 when this rule matches.
	// +kubebuilder:validation:Required
	Response ProbeResponse `json:"response"`
}

type ProbeResponse struct {
	// Status is the HTTP status code to return.
	// +kubebuilder:validation:Enum=200;204;400;401;403;404;500;502;503;504
	Status int `json:"status"`
	// Body is the response body to return.
	// +kubebuilder:validation:Required
	Body json.RawMessage `json:"body"`
}

// ProbeResponsePathMatch matches the request path (Gateway API HTTPPathMatch semantics).
type ProbeResponsePathMatch struct {
	// Type is Exact, PathPrefix, or RegularExpression. Empty defaults to PathPrefix in the resolver.
	// +optional
	// +kubebuilder:validation:Enum=Exact;PathPrefix;RegularExpression
	Type string `json:"type,omitempty"`
	// Value is the path or regular expression to match.
	// +optional
	// +kubebuilder:validation:MaxLength=1024
	Value string `json:"value,omitempty"`
}

// ProbeResponseHeaderMatch matches one request header.
type ProbeResponseHeaderMatch struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=256
	Name string `json:"name"`
	// Type is Exact or RegularExpression. Empty defaults to Exact in the resolver.
	// +optional
	// +kubebuilder:validation:Enum=Exact;RegularExpression
	Type string `json:"type,omitempty"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=4096
	Value string `json:"value"`
}

// ProbeResponseQueryParamMatch matches one query parameter.
type ProbeResponseQueryParamMatch struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=256
	Name string `json:"name"`
	// +optional
	// +kubebuilder:validation:Enum=Exact;RegularExpression
	Type string `json:"type,omitempty"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=1024
	Value string `json:"value"`
}

func (es *ElastiServiceSpec) GetScaleTargetRef() ScaleTargetRef {
	// NOTE: Required for backwards compatibility, since so far, we have been using "deployments" instead of "Deployment" in exisiting
	// CRD files. Since calse doesn't recognize "deployments" as a valid kind, we need to convert it to "Deployment".
	// We can remove it once we have migrated all the existing CRD files to use "Deployment" instead of "deployments".
	switch es.ScaleTargetRef.Kind {
	case "deployments":
		es.ScaleTargetRef.Kind = "Deployment"
	case "rollouts":
		es.ScaleTargetRef.Kind = "Rollout"
	default:
		return es.ScaleTargetRef
	}

	return es.ScaleTargetRef
}

type ScaleTargetRef struct {
	// API version of the target resource
	// +kubebuilder:validation:Enum=apps/v1;argoproj.io/v1alpha1
	APIVersion string `json:"apiVersion"`
	// Kind of the target resource
	// +kubebuilder:validation:Enum=deployments;rollouts;Deployment;StatefulSet;Rollout
	Kind string `json:"kind"`
	// Name of the target resource
	Name string `json:"name"`
}

type ElastiServiceStatus struct {
	// Last time the ElastiService was reconciled
	LastReconciledTime metav1.Time `json:"lastReconciledTime,omitempty"`
	// Last time the ElastiService was scaled up
	LastScaledUpTime *metav1.Time `json:"lastScaledUpTime,omitempty"`
	// Current mode of the ElastiService, either "proxy" or "serve".
	// "proxy" mode is when the ScaleTargetRef is scaled to 0 replicas.
	// "serve" mode is when the ScaleTargetRef is scaled to at least 1 replica.
	Mode string `json:"mode,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ElastiService is the Schema for the elastiservices API
type ElastiService struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ElastiServiceSpec   `json:"spec,omitempty"`
	Status ElastiServiceStatus `json:"status,omitempty"`
}

func (es *ElastiService) GetSpec() ElastiServiceSpec {
	es.Spec.ScaleTargetRef = es.Spec.GetScaleTargetRef()
	return es.Spec
}

//+kubebuilder:object:root=true

// ElastiServiceList contains a list of ElastiService
type ElastiServiceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ElastiService `json:"items"`
}

type ScaleTrigger struct {
	// Type of the trigger, currently only prometheus is supported
	// +kubebuilder:validation:Enum=prometheus
	Type string `json:"type"`
	// Metadata like query, serverAddress, threshold, uptimeFilter etc.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

type AutoscalerSpec struct {
	// +kubebuilder:validation:Enum=hpa;keda
	Type string `json:"type"`
	Name string `json:"name"`
}

func init() {
	SchemeBuilder.Register(&ElastiService{}, &ElastiServiceList{})
}
