package argo

const (
	typBits = 8
	numBits = 8
	sizBits = 14
	dirBits = 2

	typMask = (1 << typBits) - 1
	numMask = (1 << numBits) - 1
	sizMask = (1 << sizBits) - 1
	dirMask = (1 << dirBits) - 1

	dirNone  = 0
	dirWrite = 1
	dirRead  = 2

	numShift = 0
	typShift = numShift + numBits
	sizShift = typShift + typBits
	dirShift = sizShift + sizBits
)

func ioc(dir, t, nr, size uintptr) uintptr {
	return (dir << dirShift) | (t << typShift) | (nr << numShift) | (size << sizShift)
}

func ior(t, nr, size uintptr) uintptr {
	return ioc(dirRead, t, nr, size)
}

func iow(t, nr, size uintptr) uintptr {
	return ioc(dirWrite, t, nr, size)
}

func iowr(t, nr, size uintptr) uintptr {
	return ioc(dirRead|dirWrite, t, nr, size)
}
