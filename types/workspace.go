package types

// WorkspaceInfo is a compositor-agnostic representation of workspace state
type WorkspaceInfo struct {
	Current int   `json:"current_workspace"`
	List    []int `json:"workspace_list"`
}
