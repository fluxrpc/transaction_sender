package transaction_sender

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
	Epoch        uint64 `json:"epoch"`
	SlotIndex    uint64 `json:"slotIndex"`
	AbsoluteSlot uint64 `json:"absoluteSlot"`
	SlotsInEpoch uint64 `json:"slotsInEpoch"`
}

type SlotMessage struct {
	Params struct {
		Result struct {
			Parent uint64 `json:"parent"`
			Slot   uint64 `json:"slot"`
		} `json:"result"`
		Subscription uint64 `json:"subscription"`
	} `json:"params"`
}
