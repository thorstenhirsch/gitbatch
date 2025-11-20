package load

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/thorstenhirsch/gitbatch/internal/git"
)

// SyncLoad initializes the go-git's repository objects with given
// slice of paths. since this job is done parallel, the order of the directories
// is not kept
func SyncLoad(directories []string) (entities []*git.Repository, err error) {
	if len(directories) == 0 {
		return nil, fmt.Errorf("no directories provided")
	}

	// Use a worker pool pattern instead of unlimited goroutines
	maxWorkers := runtime.GOMAXPROCS(0)
	if len(directories) < maxWorkers {
		maxWorkers = len(directories)
	}

	// Channels for work distribution and result collection
	jobs := make(chan string, len(directories))
	results := make(chan *git.Repository, len(directories))
	errors := make(chan error, len(directories))

	// Start workers
	var wg sync.WaitGroup
	for w := 0; w < maxWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for dir := range jobs {
				entity, err := git.InitializeRepo(dir)
				if err != nil {
					errors <- err
					continue
				}
				// Initialize modtime
				entity.RefreshModTime()
				results <- entity
			}
		}()
	}

	// Send work to workers
	for _, dir := range directories {
		jobs <- dir
	}
	close(jobs)

	// Wait for all workers to finish
	go func() {
		wg.Wait()
		close(results)
		close(errors)
	}()

	// Collect results
	entities = make([]*git.Repository, 0, len(directories))
	var errCount int

	for {
		select {
		case entity, ok := <-results:
			if !ok {
				results = nil
			} else {
				entities = append(entities, entity)
			}
		case err, ok := <-errors:
			if !ok {
				errors = nil
			} else if err != nil {
				errCount++
				// Log error but continue processing other repositories
			}
		}

		if results == nil && errors == nil {
			break
		}
	}

	if len(entities) == 0 {
		return entities, fmt.Errorf("there are no git repositories at given path(s)")
	}
	return entities, nil
}
