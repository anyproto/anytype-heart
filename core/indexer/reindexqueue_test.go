package indexer

import "testing"

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestReindexTaskIdParse(t *testing.T) {
	testCases := []struct {
		input            string
		expectedSpaceId  string
		expectedRetryNum int
	}{
		{"space1", "space1", 0},
		{"space1#1", "space1", 1},
		{"space2#2", "space2", 2},
		{"space3#abc", "space3", 0}, // Invalid retry number
		{"space4#", "space4", 0},    // Empty retry number
		{"", "", 0},
	}

	for _, tc := range testCases {
		spaceId, retryNum := reindexTaskId(tc.input).Parse()
		if spaceId != tc.expectedSpaceId || retryNum != tc.expectedRetryNum {
			t.Errorf("Parse(%q) = (%q, %d); want (%q, %d)", tc.input, spaceId, retryNum, tc.expectedSpaceId, tc.expectedRetryNum)
		}
	}
}

func TestTaskPrioritySorter(t *testing.T) {
	i := &indexer{
		spacesPriority: []string{"space1", "space2", "space3"},
	}

	testCases := []struct {
		name     string
		taskIds  []string
		expected []string
	}{
		{
			name: "Different retry attempts",
			taskIds: []string{
				"space1#2",
				"space2#1",
				"space3#3",
				"space4#0",
				"space5#1",
			},
			expected: []string{
				"space4#0", // try=0, space not in priority list
				"space2#1", // try=1, index=1
				"space5#1", // try=1, index=-1 (space5 not in priority)
				"space1#2", // try=2, index=0
				"space3#3", // try=3, index=2
			},
		},
		{
			name: "Same retry attempts, different priorities",
			taskIds: []string{
				"space3#1",
				"space1#1",
				"space4#1",
				"space2#1",
			},
			expected: []string{
				"space1#1", // index=0
				"space2#1", // index=1
				"space3#1", // index=2
				"space4#1", // index=-1
			},
		},
		{
			name: "Spaces not in priority list",
			taskIds: []string{
				"space4#0",
				"space5#0",
				"space6#0",
			},
			expected: []string{
				"space4#0",
				"space5#0",
				"space6#0",
			}, // Should be sorted alphabetically among themselves
		},
		{
			name: "Mixed retry attempts and priorities",
			taskIds: []string{
				"space2#0",
				"space4#0",
				"space1#1",
				"space5#1",
				"space3#0",
			},
			expected: []string{
				"space2#0", // try=0, index=1
				"space3#0", // try=0, index=2
				"space4#0", // try=0, index=-1
				"space1#1", // try=1, index=0
				"space5#1", // try=1, index=-1
			},
		},
		{
			name: "Tasks without retries",
			taskIds: []string{
				"space3",
				"space2",
				"space4",
				"space1",
			},
			expected: []string{
				"space1", // try=0, index=0
				"space2", // try=0, index=1
				"space3", // try=0, index=2
				"space4", // try=0, index=-1
			},
		},
		{
			name: "Equal tries and no priority",
			taskIds: []string{
				"space4#1",
				"space5#1",
				"space6#1",
			},
			expected: []string{
				"space4#1",
				"space5#1",
				"space6#1",
			}, // Should be sorted alphabetically among themselves
		},
		{
			name: "Complex mix",
			taskIds: []string{
				"space3#2",
				"space1#0",
				"space4#0",
				"space2#2",
				"space5#1",
				"space2#0",
				"space1#1",
			},
			expected: []string{
				"space1#0", // try=0, index=0
				"space2#0", // try=0, index=1
				"space4#0", // try=0, index=-1
				"space1#1", // try=1, index=0
				"space5#1", // try=1, index=-1
				"space2#2", // try=2, index=1
				"space3#2", // try=2, index=2
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			taskIdsCopy := make([]string, len(tc.taskIds))
			copy(taskIdsCopy, tc.taskIds)
			i.reindexTasksSorter(taskIdsCopy)
			if !slicesEqual(taskIdsCopy, tc.expected) {
				t.Errorf("taskPrioritySorter(%v) = %v; want %v", tc.taskIds, taskIdsCopy, tc.expected)
			}
		})
	}
}
