package domain

type Mode string

const (
	ModeStd    Mode = "std"
	ModeLax    Mode = "lax"
	ModeYolo   Mode = "yolo"
	ModeStrict Mode = "strict"
)

type WorkspaceMode string

const (
	WorkspaceModeMount WorkspaceMode = "mount"
	WorkspaceModeCopy  WorkspaceMode = "copy"
)

type Profile string

type AddonName string

type WorkspaceRef struct {
	Input      string
	Dir        string
	RepoRoot   string
	ConfigPath string
	Slug       string
	Hash       string
}

type SandConfig struct {
	Profile       Profile
	Mode          Mode
	WorkspaceMode WorkspaceMode
	Addons        []AddonName

	PythonVersion string
	UVVersion     string
	GoVersion     string
	RustVersion   string

	UnknownKeys []string
}

type ResolvedConfig struct {
	Workspace       WorkspaceRef
	Config          SandConfig
	Warnings        []string
	ProfileExplicit bool
	ModeExplicit    bool
}

type ContainerIdentity struct {
	Name       string
	LegacyName string
}

type ContainerState struct {
	Exists        bool
	Running       bool
	Mode          Mode
	WorkspaceMode WorkspaceMode
}

type AddonSpec struct {
	Name           AddonName
	Script         string
	Description    string
	EnabledModes   map[Mode]bool
	RunAs          string
	HelperCommands []string
}
