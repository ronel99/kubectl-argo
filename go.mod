module kubectl-argo

go 1.22

require (
	github.com/argoproj/argo-cd/v2 v2.12.3
	github.com/spf13/cobra v1.8.1
	github.com/spf13/viper v1.19.0
	k8s.io/apimachinery v0.29.6
	k8s.io/cli-runtime v0.29.6
)

replace (
	github.com/go-check/check => github.com/go-check/check v0.0.0-20180628173108-788fd7840127
	github.com/go-telegram-bot-api/telegram-bot-api/v5 => github.com/OvyFlash/telegram-bot-api/v5 v5.0.0-20240108230938-63e5c59035bf
	github.com/golang/protobuf => github.com/golang/protobuf v1.5.4
	github.com/grpc-ecosystem/grpc-gateway => github.com/grpc-ecosystem/grpc-gateway v1.16.0
	gopkg.in/yaml.v2 => gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 => gopkg.in/yaml.v3 v3.0.1

	k8s.io/api => k8s.io/api v0.29.6
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.29.6
	k8s.io/apimachinery => k8s.io/apimachinery v0.29.6
	k8s.io/apiserver => k8s.io/apiserver v0.29.6
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.29.6
	k8s.io/client-go => k8s.io/client-go v0.29.6
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.29.6
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.29.6
	k8s.io/code-generator => k8s.io/code-generator v0.29.6
	k8s.io/component-base => k8s.io/component-base v0.29.6
	k8s.io/component-helpers => k8s.io/component-helpers v0.29.6
	k8s.io/controller-manager => k8s.io/controller-manager v0.29.6
	k8s.io/cri-api => k8s.io/cri-api v0.29.6
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.29.6
	k8s.io/dynamic-resource-allocation => k8s.io/dynamic-resource-allocation v0.29.6
	k8s.io/endpointslice => k8s.io/endpointslice v0.29.6
	k8s.io/kms => k8s.io/kms v0.29.6
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.29.6
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.29.6
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.29.6
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.29.6
	k8s.io/kubectl => k8s.io/kubectl v0.29.6
	k8s.io/kubelet => k8s.io/kubelet v0.29.6
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.29.6
	k8s.io/metrics => k8s.io/metrics v0.29.6
	k8s.io/mount-utils => k8s.io/mount-utils v0.29.6
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.29.6
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.29.6
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.29.6
	k8s.io/sample-controller => k8s.io/sample-controller v0.29.6
)
