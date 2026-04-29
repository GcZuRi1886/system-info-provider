package types

type BrightnessInfo struct {
	Current int     `json:"current"`
	Maximum int     `json:"maximum"`
	Percentage   float64 `json:"percentage"` // 0.0–1.0
}
