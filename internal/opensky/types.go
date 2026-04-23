package opensky

// StateResponse is the JSON response from /states/all.
type StateResponse struct {
	Time   int64           `json:"time"`
	States [][]interface{} `json:"states"`
}
