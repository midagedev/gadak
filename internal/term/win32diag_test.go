package term

import (
	"encoding/binary"
	"testing"
	"unsafe"
)

// The job-list decode contract, pinned against the Windows ABI rather
// than against this package's own idea of it (GDK-891). The CI failure
// that opened this round — job members reading [] with a live shell in
// the job — was a mirror of JOBOBJECT_BASIC_PROCESS_ID_LIST whose count
// fields were uint32 where the ABI says ULONG_PTR: on x64 the in-list
// count was read from the high half of the assigned count, which is
// always zero. So these tests build the bytes the kernel writes with
// their own hands — pointer-sized little-endian words, independent of
// any struct this package declares — and the decoder must read them
// back. They run on every platform for exactly that reason: a layout
// only CI executes is a layout nothing verifies until Windows sees it.

// encodeJobList builds the raw bytes QueryInformationJobObject fills for
// JobObjectBasicProcessIdList: two ULONG_PTR counts then the pid array,
// little-endian — the ABI, written without reference to jobListWord's
// users.
func encodeJobList(assigned, inList int, pids ...uint64) []byte {
	w := int(unsafe.Sizeof(uintptr(0)))
	b := make([]byte, 0, (2+len(pids))*w)
	var head [8]byte
	if w == 8 {
		binary.LittleEndian.PutUint64(head[:8], uint64(assigned))
		b = append(b, head[:8]...)
		binary.LittleEndian.PutUint64(head[:8], uint64(inList))
		b = append(b, head[:8]...)
	} else {
		binary.LittleEndian.PutUint32(head[:4], uint32(assigned))
		b = append(b, head[:4]...)
		binary.LittleEndian.PutUint32(head[:4], uint32(inList))
		b = append(b, head[:4]...)
	}
	for _, pid := range pids {
		if w == 8 {
			binary.LittleEndian.PutUint64(head[:8], pid)
			b = append(b, head[:8]...)
		} else {
			binary.LittleEndian.PutUint32(head[:4], uint32(pid))
			b = append(b, head[:4]...)
		}
	}
	return b
}

func TestDecodeJobPidList(t *testing.T) {
	cases := []struct {
		name     string
		raw      []byte
		want     []int
		assigned int
	}{
		{
			// The CI shape: one shell in the job. A decoder that reads
			// the counts as 32-bit sees inList=0 here and answers nil —
			// the members=[] of run 36625666166.
			name:     "one member",
			raw:      encodeJobList(1, 1, 4242),
			want:     []int{4242},
			assigned: 1,
		},
		{
			name:     "shell and grandchild",
			raw:      encodeJobList(2, 2, 100, 200),
			want:     []int{100, 200},
			assigned: 2,
		},
		{
			// A zero pid in the list is skipped, not handed back.
			name:     "zero pid skipped",
			raw:      encodeJobList(3, 3, 100, 0, 300),
			want:     []int{100, 300},
			assigned: 3,
		},
		{
			// Empty job: nil, the "nothing running" fail direction.
			name:     "empty",
			raw:      encodeJobList(0, 0),
			want:     nil,
			assigned: 0,
		},
		{
			// A truncated buffer answers nil rather than reading past it.
			name: "truncated header",
			raw:  []byte{1, 0, 0},
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, assigned := decodeJobPidList(tc.raw)
			if len(got) != len(tc.want) {
				t.Fatalf("decodeJobPidList(%v) = %v (assigned %d); want %v (assigned %d)",
					tc.raw, got, assigned, tc.want, tc.assigned)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("decodeJobPidList(%v) = %v; want %v", tc.raw, got, tc.want)
				}
			}
			if assigned != tc.assigned {
				t.Fatalf("assigned = %d; want %d", assigned, tc.assigned)
			}
		})
	}
}

// TestJobListBytesPinsTheHeaderSize guards the sizing math against the
// same drift: the query buffer must hold two pointer-sized counts plus
// the pids, or the first query is short a header word and the decode
// reads pid bytes as counts.
func TestJobListBytesPinsTheHeaderSize(t *testing.T) {
	if got := jobListBytes(0); got != 2*jobListWord {
		t.Fatalf("jobListBytes(0) = %d; want %d (two pointer-sized counts)", got, 2*jobListWord)
	}
	if got := jobListBytes(jobListInlinePids); got != (2+jobListInlinePids)*jobListWord {
		t.Fatalf("jobListBytes(%d) = %d; want %d", jobListInlinePids, got, (2+jobListInlinePids)*jobListWord)
	}
}
