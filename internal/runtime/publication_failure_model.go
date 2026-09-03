package runtime

func isPublicationFailureState(version string) bool {
	return version == CorePublicationFailureStateVersion || isActionIntentState(version)
}
