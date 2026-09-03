package runtime

func isPublicationNewOnlyState(version string) bool {
	return version == CorePublicationNewOnlyStateVersion || isPublicationFailureState(version)
}
