package buildrun

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	buildv1alpha1 "github.com/shipwright-io/build/pkg/apis/build/v1alpha1"
	"github.com/shipwright-io/cli/pkg/shp/cmd/runner"
	"github.com/shipwright-io/cli/pkg/shp/params"
)

// CancelCommand contains data input from user for delete sub-command
type CancelCommand struct {
	cmd *cobra.Command

	name string
}

func cancelCmd() runner.SubCommand {
	return &CancelCommand{
		cmd: &cobra.Command{
			Use:   "cancel <name>",
			Short: "Cancel BuildRun",
			Args:  cobra.ExactArgs(1),
		},
	}
}

// Cmd returns cobra command object
func (c *CancelCommand) Cmd() *cobra.Command {
	return c.cmd
}

// Complete fills in data provided by user
func (c *CancelCommand) Complete(params *params.Params, args []string) error {
	c.name = args[0]

	return nil
}

// Validate validates data input by user
func (c *CancelCommand) Validate() error {
	return nil
}

// Run executes cancel sub-command logic
func (c *CancelCommand) Run(params *params.Params, ioStreams *genericclioptions.IOStreams) error {
	clientset, err := params.ShipwrightClientSet()
	if err != nil {
		return err
	}

	br := &buildv1alpha1.BuildRun{}
	if br, err = clientset.ShipwrightV1alpha1().BuildRuns(params.Namespace()).Get(c.cmd.Context(), c.name, metav1.GetOptions{}); err != nil {
		return fmt.Errorf("failed to retrieve BuildRun %s: %s", c.name, err.Error())
	}
	//TODO replace with br.IsDone() when that is available and vendored in
	cond := br.Status.GetCondition(buildv1alpha1.Succeeded)
	if cond != nil && cond.GetStatus() != corev1.ConditionUnknown {
		return fmt.Errorf("failed to cancel BuildRun %s: execution has already finished", c.name)
	}

	//TODO use constant when vendor in api changes
	type patchStringValue struct {
		Op    string `json:"op"`
		Path  string `json:"path"`
		Value string `json:"value"`
	}
	payload := []patchStringValue{{
		Op:    "replace",
		Path:  "/spec/state",
		Value: buildv1alpha1.BuildRunStateCancel,
	}}
	var data []byte
	if data, err = json.Marshal(payload); err != nil {
		return err
	}
	if _, err = clientset.ShipwrightV1alpha1().BuildRuns(params.Namespace()).Patch(c.Cmd().Context(), c.name, types.JSONPatchType, data, metav1.PatchOptions{}); err != nil {
		return err
	}

	fmt.Fprintf(ioStreams.Out, "BuildRun successfully canceled '%v'\n", c.name)

	return nil
}
