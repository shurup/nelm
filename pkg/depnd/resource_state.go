package depnd

type ResourceState string

const (
	ResourceStateAbsent  ResourceState = "absent"
	ResourceStatePresent ResourceState = "present"
	ResourceStateReady   ResourceState = "ready"
)
