package config

type RolloutConfig struct {
	RolloutID               string
	RolloutPhaseID          string
	StableModelVersionID    string
	CandidateModelVersionID string
	CandidatePercentage     int
	FeatureFlagKey          string
	StreamKey               string
	StreamConsumerGroup     string
	StreamConsumerName      string
}

type RolloutPolicy struct {
	// Guard settings
	MinRequestsBeforeGuard  int
	FreshWindowSize         int
	FreshWindowMaxErrorRate float64
	AbsoluteMaxErrorRate    float64

	// Controller (2-min cron) settings
	ControllerIntervalSecs int
	AdvanceMinRequests     int
	AdvanceMaxErrorRate    float64
	AdvanceMaxP95LatencyMs int
}

func DefaultConfig() RolloutConfig {
	return RolloutConfig{
		RolloutID:               "rollout-001",
		RolloutPhaseID:          "phase-1",
		StableModelVersionID:    "model-v1",
		CandidateModelVersionID: "model-v2",
		CandidatePercentage:     10,
		FeatureFlagKey:          "feature-flag:model-routing:development",
		StreamKey:               "telemetry:inference-completed",
		StreamConsumerGroup:     "rollout-controller",
		StreamConsumerName:      "controller-1",
	}
}

func DefaultPolicy() RolloutPolicy {
	return RolloutPolicy{
		MinRequestsBeforeGuard:  50,
		FreshWindowSize:         50,
		FreshWindowMaxErrorRate: 0.30,
		AbsoluteMaxErrorRate:    0.05,
		ControllerIntervalSecs:  120,
		AdvanceMinRequests:      100,
		AdvanceMaxErrorRate:     0.02,
		AdvanceMaxP95LatencyMs:  250,
	}
}
