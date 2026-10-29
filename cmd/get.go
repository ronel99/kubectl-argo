package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/duration"
	"k8s.io/cli-runtime/pkg/printers"

	"kubectl-argo/internal/argocd"
)

var (
	flagGetOutput    string
	flagGetNoHeaders bool
)

var getCmd = &cobra.Command{
	Use:   "get (pods|svc|ing) [NAME]",
	Short: "Display pods, services, or ingresses managed by Argo CD in the target namespace",
	Long: `Fetches the live resource tree from Argo CD (GET /api/v1/applications/{resolved-app-name}/resource-tree),
filters by the requested resource kind (Pod, Service, or Ingress), and renders the result
in standard kubectl table format using k8s.io/cli-runtime/pkg/printers.`,
	Example: `  # List all pods in the payments-prod namespace
  kubectl-argo get pods -n payments-prod

  # List all services in the payments-prod namespace
  kubectl-argo get svc -n payments-prod

  # List all ingresses in the payments-prod namespace with wide output
  kubectl-argo get ing -n payments-prod -o wide`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runGet,
}

func init() {
	getCmd.Flags().StringVarP(&flagGetOutput, "output", "o", "", "Output format (supports \"wide\")")
	getCmd.Flags().BoolVar(&flagGetNoHeaders, "no-headers", false, "When using the default output format, don't print headers")
	rootCmd.AddCommand(getCmd)
}

func runGet(cmd *cobra.Command, args []string) error {
	canonicalKind, err := resolveCanonicalKind(args[0])
	if err != nil {
		return err
	}

	var resourceName string
	if len(args) > 1 {
		resourceName = strings.TrimSpace(args[1])
	}

	if flagGetOutput != "" && flagGetOutput != "wide" {
		return fmt.Errorf("unsupported output format %q: supported formats are default table or \"wide\"", flagGetOutput)
	}

	client, err := ClientFromContext(cmd.Context())
	if err != nil {
		return err
	}

	resolvedApp, err := ResolvedAppFromContext(cmd.Context())
	if err != nil {
		return err
	}

	tree, err := client.FetchResourceTree(cmd.Context(), resolvedApp)
	if err != nil {
		return err
	}

	nodes := argocd.FilterResourceNodes(tree, canonicalKind, resolvedApp.TargetNamespace, resourceName)
	if len(nodes) == 0 {
		if resourceName != "" {
			return fmt.Errorf("Error from server (NotFound): %s %q not found in namespace %q", strings.ToLower(canonicalKind), resourceName, resolvedApp.TargetNamespace)
		}
		fmt.Fprintf(os.Stderr, "No resources found in %s namespace.\n", resolvedApp.TargetNamespace)
		return nil
	}

	return printResourceTable(canonicalKind, nodes, flagGetOutput == "wide", flagGetNoHeaders, os.Stdout)
}

// resolveCanonicalKind normalizes kubectl resource type shorthands to canonical Kubernetes Kind names.
func resolveCanonicalKind(input string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "po", "pod", "pods":
		return "Pod", nil
	case "svc", "service", "services":
		return "Service", nil
	case "ing", "ingress", "ingresses":
		return "Ingress", nil
	default:
		return "", fmt.Errorf("unsupported resource type %q: supported types are pods (po), svc (service), and ing (ingress)", input)
	}
}

// printResourceTable converts Argo CD ResourceNodes into a metav1.Table and prints them via k8s.io/cli-runtime/pkg/printers.
func printResourceTable(kind string, nodes []v1alpha1.ResourceNode, wide bool, noHeaders bool, out io.Writer) error {
	table := buildMetaV1Table(kind, nodes)

	printer := printers.NewTablePrinter(printers.PrintOptions{
		Wide:      wide,
		NoHeaders: noHeaders,
	})

	return printer.PrintObj(table, out)
}

func buildMetaV1Table(kind string, nodes []v1alpha1.ResourceNode) *metav1.Table {
	table := &metav1.Table{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "meta.k8s.io/v1",
			Kind:       "Table",
		},
	}

	switch kind {
	case "Pod":
		table.ColumnDefinitions = []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name"},
			{Name: "Ready", Type: "string"},
			{Name: "Status", Type: "string"},
			{Name: "Restarts", Type: "string"},
			{Name: "Age", Type: "string"},
			{Name: "Node", Type: "string", Priority: 1},
			{Name: "Health", Type: "string", Priority: 1},
		}
		for _, n := range nodes {
			ready := firstNonEmpty(nodeInfoValue(n, "Containers"), "-")
			status := firstNonEmpty(nodeInfoValue(n, "Status Reason"), healthStatusString(n))
			restarts := firstNonEmpty(nodeInfoValue(n, "Restart Count"), "0")
			age := formatNodeAge(n.CreatedAt)
			nodeName := firstNonEmpty(nodeInfoValue(n, "Node"), "<none>")
			health := healthStatusString(n)

			table.Rows = append(table.Rows, metav1.TableRow{
				Cells: []interface{}{n.Name, ready, status, restarts, age, nodeName, health},
			})
		}

	case "Service":
		table.ColumnDefinitions = []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name"},
			{Name: "Type", Type: "string"},
			{Name: "Cluster-IP", Type: "string"},
			{Name: "External-IP", Type: "string"},
			{Name: "Port(s)", Type: "string"},
			{Name: "Age", Type: "string"},
			{Name: "Health", Type: "string", Priority: 1},
		}
		for _, n := range nodes {
			svcType := firstNonEmpty(nodeInfoValue(n, "Type"), "ClusterIP")
			clusterIP := firstNonEmpty(nodeInfoValue(n, "ClusterIP", "Cluster-IP"), "<none>")
			externalIP := firstNonEmpty(extractLoadBalancerIngress(n), nodeInfoValue(n, "ExternalIP", "External-IP"), "<none>")
			ports := firstNonEmpty(nodeInfoValue(n, "Ports", "Port(s)"), "<none>")
			age := formatNodeAge(n.CreatedAt)
			health := healthStatusString(n)

			table.Rows = append(table.Rows, metav1.TableRow{
				Cells: []interface{}{n.Name, svcType, clusterIP, externalIP, ports, age, health},
			})
		}

	case "Ingress":
		table.ColumnDefinitions = []metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name"},
			{Name: "Hosts", Type: "string"},
			{Name: "Address", Type: "string"},
			{Name: "Age", Type: "string"},
			{Name: "Health", Type: "string", Priority: 1},
		}
		for _, n := range nodes {
			hosts := firstNonEmpty(extractExternalURLs(n), nodeInfoValue(n, "Hosts", "Host"), "*")
			address := firstNonEmpty(extractLoadBalancerIngress(n), "<none>")
			age := formatNodeAge(n.CreatedAt)
			health := healthStatusString(n)

			table.Rows = append(table.Rows, metav1.TableRow{
				Cells: []interface{}{n.Name, hosts, address, age, health},
			})
		}
	}

	return table
}

func nodeInfoValue(node v1alpha1.ResourceNode, keys ...string) string {
	for _, key := range keys {
		for _, item := range node.Info {
			if strings.EqualFold(strings.TrimSpace(item.Name), key) && strings.TrimSpace(item.Value) != "" {
				return strings.TrimSpace(item.Value)
			}
		}
	}
	return ""
}

func healthStatusString(node v1alpha1.ResourceNode) string {
	if node.Health != nil && node.Health.Status != "" {
		return string(node.Health.Status)
	}
	return "Unknown"
}

func extractLoadBalancerIngress(node v1alpha1.ResourceNode) string {
	if node.NetworkingInfo == nil || len(node.NetworkingInfo.Ingress) == 0 {
		return ""
	}
	var addrs []string
	for _, ing := range node.NetworkingInfo.Ingress {
		if ing.IP != "" {
			addrs = append(addrs, ing.IP)
		} else if ing.Hostname != "" {
			addrs = append(addrs, ing.Hostname)
		}
	}
	return strings.Join(addrs, ",")
}

func extractExternalURLs(node v1alpha1.ResourceNode) string {
	if node.NetworkingInfo == nil || len(node.NetworkingInfo.ExternalURLs) == 0 {
		return ""
	}
	return strings.Join(node.NetworkingInfo.ExternalURLs, ",")
}

func formatNodeAge(createdAt *metav1.Time) string {
	if createdAt == nil || createdAt.IsZero() {
		return "<unknown>"
	}
	return duration.HumanDuration(time.Since(createdAt.Time))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
