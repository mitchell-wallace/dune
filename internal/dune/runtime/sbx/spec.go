package sbx

import "fmt"

type Spec struct {
	InstanceName      string
	WorkspaceHostPath string
	Profile           string
	TemplateRef       string
	WorkingDir        string
	Shell             string
	Timezone          string
}

func InstanceName(workspaceSlug, profile string) string {
	return fmt.Sprintf("dune-%s-%s", workspaceSlug, profile)
}
