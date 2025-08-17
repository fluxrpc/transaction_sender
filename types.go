package solana_transaction_sender

type getEpochInfoResponse struct {
	Result epochInfo `json:"result"`
}
type getSlotResponse struct {
	Result uint64 `json:"result"`
}

type getLeaderScheduleResponse struct {
	Result map[string][]uint64 `json:"result"`
}

type getClusterNodesResponse struct {
	Result []*Leader `json:"result"`
}

type Leader struct {
	PubKey          string `json:"pubkey"`
	TPU             string `json:"tpu"`
	TPUForwards     string `json:"tpuForwards"`
	TPUQuic         string `json:"tpuQuic"`
	TPUForwardsQuic string `json:"tpuForwardsQuic"`
}

type epochInfo struct {
	SlotIndex    uint64 `json:"slotIndex"`
	AbsoluteSlot uint64 `json:"absoluteSlot"`
	SlotsInEpoch uint64 `json:"slotsInEpoch"`
}
