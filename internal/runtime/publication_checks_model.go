package runtime

func isPublicationChecksState(version string) bool {
	return atLeast(version, CorePublicationChecksStateVersion)
}
