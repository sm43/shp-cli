package build

import (
	"errors"
	"fmt"
	buildv1alpha1 "github.com/shipwright-io/build/pkg/apis/build/v1alpha1"
	"github.com/shipwright-io/cli/pkg/shp/cmd/pod"
	"github.com/shipwright-io/cli/pkg/shp/cmd/runner"
	"github.com/shipwright-io/cli/pkg/shp/cmd/taskrun"
	"github.com/shipwright-io/cli/pkg/shp/flags"
	"github.com/shipwright-io/cli/pkg/shp/params"
	"github.com/shipwright-io/cli/pkg/shp/resource"
	"github.com/spf13/cobra"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// RunCommand represents the `build run` sub-command, which creates a unique BuildRun instance to run
// the build process, informed via arguments.
type RunCommand struct {
	cmd *cobra.Command // cobra command instance

	buildName    string                      // build name
	buildRunSpec *buildv1alpha1.BuildRunSpec // stores command-line flags
	Follow       bool
}

const buildRunLongDesc = `
Creates a unique BuildRun instance for the given Build, which starts the build
process orchestrated by the Shipwright build controller. For example:

	$ shp build run my-app
`

// Cmd returns cobra.Command object of the create sub-command.
func (r *RunCommand) Cmd() *cobra.Command {
	return r.cmd
}

// Complete picks the build resource name from arguments, or error when not informed.
func (r *RunCommand) Complete(params *params.Params, args []string) error {
	switch len(args) {
	case 1:
		r.buildName = args[0]
	default:
		return errors.New("Build name is not informed")
	}
	// overwriting build-ref name to use what's on arguments
	return r.Cmd().Flags().Set(flags.BuildrefNameFlag, r.buildName)
}

// Validate the user must inform the build resource name.
func (r *RunCommand) Validate() error {
	if r.buildName == "" {
		return fmt.Errorf("name is not informed")
	}
	return nil
}

// Run creates a BuildRun resource based on Build's name informed on arguments.
func (r *RunCommand) Run(params *params.Params, ioStreams *genericclioptions.IOStreams) error {
	// resource using GenerateName, which will provice a unique instance
	br := &buildv1alpha1.BuildRun{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-", r.buildName),
		},
		Spec: *r.buildRunSpec,
	}
	flags.SanitizeBuildRunSpec(&br.Spec)

	buildRunResource := resource.GetBuildRunResource(params)
	if err := buildRunResource.Create(r.cmd.Context(), "", br); err != nil {
		return err
	}

	if !r.Follow {
		fmt.Fprintf(ioStreams.Out, "BuildRun created %q for build %q\n", br.GetName(), r.buildName)
		return nil
	}

	br, err := waitForBuildRunToHaveTaskRun(r.cmd.Context(), br.Name, buildRunResource, ioStreams)
	if err != nil {
		return err
	}
	if br == nil {
		// not expected, but sanity check to avoid panic
		return fmt.Errorf("build run watch function exitted unexpectedly")
	}

	if br.ObjectMeta.DeletionTimestamp != nil {
		return fmt.Errorf("build run %s was deleted before it terminated", br.Name)
	}

	if br.Status.LatestTaskRunRef == nil {
		return fmt.Errorf("build run %s terminated before task run ref was set, inspect build run status for details", br.Name)
	}

	taskRunResource := resource.GetTaskRunResource(params)
	taskRunName := *br.Status.LatestTaskRunRef

	tr, err := taskrun.WaitForTaskRunToHavePod(r.cmd.Context(), taskRunName, taskRunResource, ioStreams)
	if err != nil {
		return err
	}
	if tr == nil {
		return fmt.Errorf("task run watch function exitted unexpectedly")
	}
	if tr.ObjectMeta.DeletionTimestamp != nil {
		return fmt.Errorf("task run %s was deleted before it terminated", tr.Name)
	}
	podName := tr.Status.PodName

	kube, err := params.ClientSet()
	if err != nil {
		return fmt.Errorf("could not build k8s client: %s", err.Error())
	}
	pod.Tail(podName, taskRunName, br.Namespace, kube, ioStreams)
	return nil

	/*tparams := &tkncli.TektonParams{}
	tparams.SetNamespace(br.Namespace)
	logOpts := getTKNLogOpts(tparams, ioStreams, *br.status.LatestTaskRunRef)

	return Tail(logOpts)*/
}

// runCmd instantiate the "build run" sub-command using common BuildRun flags.
func runCmd() runner.SubCommand {
	cmd := &cobra.Command{
		Use:   "run <name>",
		Short: "Start a build specified by 'name'",
		Long:  buildRunLongDesc,
	}
	runCommand := &RunCommand{
		cmd:          cmd,
		buildRunSpec: flags.BuildRunSpecFromFlags(cmd.Flags()),
	}
	cmd.Flags().BoolVarP(&runCommand.Follow, "follow", "F", runCommand.Follow, "Start a build and watch its log until it completes or fails.")
	return runCommand
}
