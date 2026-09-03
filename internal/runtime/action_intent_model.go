package runtime

func isActionIntentState(version string) bool {
	return version == CoreActionIntentStateVersion || isActionAdmissionState(version)
}

func isActionAdmissionState(version string) bool {
	return version == CoreActionAdmissionStateVersion || isActionGrantAdmissionState(version) || isActionDeliveryState(version)
}

func isActionGrantAdmissionState(version string) bool {
	return version == CoreActionGrantAdmissionStateVersion || isActionDeliveryState(version)
}

func isActionDeliveryState(version string) bool {
	return version == CoreActionDeliveryStateVersion || isForkState(version)
}

func isForkState(version string) bool {
	return version == CoreForkStateVersion || isWorkspaceState(version)
}

func isWorkspaceState(version string) bool {
	return version == CoreWorkspaceStateVersion || isWorkspaceTreeState(version) || isDecisionState(version)
}

func isWorkspaceTreeState(version string) bool {
	return version == CoreWorkspaceTreeStateVersion
}
