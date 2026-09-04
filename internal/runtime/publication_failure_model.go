package runtime

func isPublicationFailureState(version string) bool {
	return atLeast(version, CorePublicationFailureStateVersion)
}
