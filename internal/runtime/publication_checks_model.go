package runtime

func isPublicationChecksState(version string) bool {
	return version == CorePublicationChecksStateVersion || isPublicationNewOnlyState(version)
}
