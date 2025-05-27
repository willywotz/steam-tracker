package steamtracker

func setOptional[T any](value *T, add func(v T)) {
	if value != nil {
		add(*value)
	}
}
