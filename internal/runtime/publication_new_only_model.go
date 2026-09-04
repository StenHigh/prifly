package runtime

func isPublicationNewOnlyState(version string) bool {
	return atLeast(version, CorePublicationNewOnlyStateVersion)
}
