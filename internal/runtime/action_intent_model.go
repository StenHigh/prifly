package runtime

func isActionIntentState(version string) bool { return atLeast(version, CoreActionIntentStateVersion) }

func isActionAdmissionState(version string) bool {
	return atLeast(version, CoreActionAdmissionStateVersion)
}

func isActionGrantAdmissionState(version string) bool {
	return atLeast(version, CoreActionGrantAdmissionStateVersion)
}

func isActionDeliveryState(version string) bool {
	return atLeast(version, CoreActionDeliveryStateVersion)
}

func isForkState(version string) bool { return atLeast(version, CoreForkStateVersion) }

func isWorkspaceState(version string) bool { return atLeast(version, CoreWorkspaceStateVersion) }

func isWorkspaceTreeState(version string) bool {
	return atLeast(version, CoreWorkspaceTreeStateVersion)
}
