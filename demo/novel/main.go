package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/feiskyer/swarm-go"
)

// Event types
const (
	EventOutline swarm.EventType = "OutlineEvent"
	EventChapter swarm.EventType = "ChapterEvent"
	EventNovel   swarm.EventType = "NovelEvent"
)

// OutlineEvent represents a novel outline
type OutlineEvent struct {
	*swarm.BaseEvent
	Topic    string   `json:"topic"`
	Chapters []string `json:"chapters"`
}

// NewOutlineEvent creates a new OutlineEvent
func NewOutlineEvent(topic string, chapters []string) *OutlineEvent {
	return swarm.NewEvent(EventOutline, OutlineEvent{
		Topic:    topic,
		Chapters: chapters,
	})
}

// WriteChapterTask represents a task to write a chapter
type WriteChapterTask struct {
	Topic   string `json:"topic"`
	Title   string `json:"title"`
	Chapter int    `json:"chapter"`
}

// ChapterEvent represents a written chapter
type ChapterEvent struct {
	*swarm.BaseEvent
	Title   string `json:"title"`
	Content string `json:"content"`
}

// NewChapterEvent creates a new ChapterEvent
func NewChapterEvent(title, content string) *ChapterEvent {
	return swarm.NewEvent(EventChapter, ChapterEvent{
		Title:   title,
		Content: content,
	})
}

// NovelEvent represents the complete novel
type NovelEvent struct {
	*swarm.BaseEvent
	Topic    string            `json:"topic"`
	Chapters map[string]string `json:"chapters"`
}

// NewNovelEvent creates a new NovelEvent
func NewNovelEvent(topic string, chapters map[string]string) *NovelEvent {
	return swarm.NewEvent(EventNovel, NovelEvent{
		Topic:    topic,
		Chapters: chapters,
	})
}

func handleStartEvent(ctx *swarm.Context, event swarm.Event, client *swarm.Swarm) (swarm.Event, error) {
	if event.Type() != swarm.EventStart {
		return nil, fmt.Errorf("expected start event, got %s", event.Type())
	}

	eventData := event.Data()
	if eventData == nil {
		return nil, fmt.Errorf("no event data received")
	}

	topicVal, ok := eventData["topic"]
	if !ok {
		return nil, fmt.Errorf("topic not found in event data")
	}

	topic, ok := topicVal.(string)
	if !ok {
		return nil, fmt.Errorf("invalid topic type in event data")
	}
	ctx.Set("topic", topic)

	outlineAgent := swarm.NewAgent("Outline Creator").WithInstructions(`
		You are a creative writer tasked with creating novel outlines. When given a topic:
		1. Create a 3-chapter outline where each chapter has a clear focus and advances the story
		2. Return only the chapter titles, one per line
		Keep the titles concise but descriptive.
	`)
	messages := []map[string]interface{}{
		{
			"role":    "user",
			"content": fmt.Sprintf("Create a 3-chapter outline for a short novel about %s.", topic),
		},
	}

	response, err := client.Run(ctx.Context(), outlineAgent, messages, nil, "gpt-4", false, false, 10, true)
	if err != nil {
		return nil, fmt.Errorf("failed to generate outline: %w", err)
	}

	if len(response.Messages) == 0 {
		return nil, fmt.Errorf("no response messages received")
	}

	lastMsg := response.Messages[len(response.Messages)-1]
	content, ok := lastMsg["content"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid response content type")
	}

	outline := content
	var chapters []string
	for _, line := range strings.Split(outline, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			// Remove any chapter numbers if they exist in the outline
			line = strings.TrimLeft(line, "0123456789. ")
			chapters = append(chapters, line)
		}
	}

	ctx.Set("chapters", chapters)
	return NewOutlineEvent(topic, chapters), nil
}

func handleOutlineEvent(ctx *swarm.Context, event swarm.Event) (swarm.Event, error) {
	outlineEvent := event.(*OutlineEvent)

	var tasks []swarm.Task
	for i, chapter := range outlineEvent.Chapters {
		tasks = append(tasks, swarm.NewTask(
			fmt.Sprintf("chapter-%d", i+1),
			swarm.EventType("WriteChapter"),
			&WriteChapterTask{
				Topic:   outlineEvent.Topic,
				Title:   chapter,
				Chapter: i + 1,
			},
		).WithPriority(i).WithTimeout(10*time.Minute))
	}

	parallelEvent, err := swarm.NewParallelEvent(tasks, "OutlineEvent")
	if err != nil {
		return nil, fmt.Errorf("failed to create parallel event: %w", err)
	}
	return parallelEvent, nil
}

func handleWriteChapter(ctx *swarm.Context, event swarm.Event, client *swarm.Swarm) (swarm.Event, error) {
	writeTask := WriteChapterTask{}
	if err := swarm.ToStruct(event.Data(), &writeTask); err != nil {
		return nil, fmt.Errorf("failed to unmarshal task payload: %w", err)
	}

	chapterAgent := swarm.NewAgent("Chapter Writer").WithInstructions(`
		You are a creative writer tasked with writing novel chapters. When given a chapter title and topic:
		1. Write an engaging and well-structured chapter that fits the overall story
		2. Ensure proper pacing and character development
		3. Keep the writing style consistent throughout
		Return only the chapter content without any additional commentary.
	`)

	messages := []map[string]interface{}{
		{
			"role":    "user",
			"content": fmt.Sprintf("Write chapter %d titled '%s' for a short novel about %s.", writeTask.Chapter, writeTask.Title, writeTask.Topic),
		},
	}

	response, err := client.Run(ctx.Context(), chapterAgent, messages, nil, "gpt-4", false, false, 10, true)
	if err != nil {
		return nil, fmt.Errorf("failed to write chapter: %w", err)
	}

	if len(response.Messages) == 0 {
		return nil, fmt.Errorf("no response messages received")
	}
	lastMsg := response.Messages[len(response.Messages)-1]
	content, ok := lastMsg["content"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid response content type")
	}

	chapterContent := content
	ctx.Set(fmt.Sprintf("chapter_%d", writeTask.Chapter), chapterContent)

	// Create chapter event
	chapterEvent := NewChapterEvent(writeTask.Title, chapterContent)
	chapterEvent.BaseEvent.SetData(map[string]interface{}{
		"chapter": writeTask.Chapter,
	})
	return chapterEvent, nil
}

func handleOutlineEventResult(ctx *swarm.Context, event swarm.Event) (swarm.Event, error) {
	// Check if it's a parallel result event
	if resultEvent, ok := event.(*swarm.ParallelResultEvent); ok {
		results := resultEvent.GetResults()
		errors := resultEvent.GetErrors()

		if len(errors) > 0 {
			fmt.Printf("Errors encountered:\n")
			for taskID, err := range errors {
				fmt.Printf("- Task %s: %v\n", taskID, err)
			}
		}

		// Create ordered array of chapters
		orderedChapters := make([]struct {
			Title   string
			Content string
			Chapter int
		}, 0, len(results))

		// Collect chapters with their chapter numbers
		for _, result := range results {
			if chapterEvent, ok := result.(*ChapterEvent); ok {
				data := chapterEvent.Data()
				if chapterVal, ok := data["chapter"].(int); ok {
					orderedChapters = append(orderedChapters, struct {
						Title   string
						Content string
						Chapter int
					}{
						Title:   chapterEvent.Title,
						Content: chapterEvent.Content,
						Chapter: chapterVal,
					})
				} else if chapterFloat, ok := data["chapter"].(float64); ok {
					orderedChapters = append(orderedChapters, struct {
						Title   string
						Content string
						Chapter int
					}{
						Title:   chapterEvent.Title,
						Content: chapterEvent.Content,
						Chapter: int(chapterFloat),
					})
				} else {
					fmt.Printf("Warning: Chapter %s missing chapter number (type: %T)\n", chapterEvent.Title, data["chapter"])
				}
			}
		}

		// Sort chapters by their chapter number
		sort.Slice(orderedChapters, func(i, j int) bool {
			return orderedChapters[i].Chapter < orderedChapters[j].Chapter
		})

		// Create the final chapters map with ordered titles
		chapters := make(map[string]string)
		var chapterTitles []string
		for _, chapter := range orderedChapters {
			chapters[chapter.Title] = chapter.Content
			chapterTitles = append(chapterTitles, chapter.Title)
		}

		topicVal, ok := ctx.Get("topic")
		if !ok {
			return nil, fmt.Errorf("topic not found in context")
		}
		topic := topicVal.(string)

		return swarm.NewStopEvent(map[string]interface{}{
			"topic":          topic,
			"chapters":       chapters,
			"chapter_titles": chapterTitles, // Already in correct order
		}), nil
	}

	return nil, fmt.Errorf("expected parallel result event, got %T", event)
}

func main() {
	client, err := swarm.NewDefaultSwarm()
	if err != nil {
		fmt.Printf("Failed to create client: %v\n", err)
		os.Exit(1)
	}

	// Create workflow
	workflow := swarm.NewWorkflow("novel-writer")
	workflow.WithConfig(swarm.WorkflowConfig{
		Name:       "novel-writer",
		Timeout:    30 * time.Minute,
		Verbose:    true,
		MaxTurns:   10,
		MaxRetries: 3,
	})

	// Add step to generate outline
	outlineStep := swarm.NewStep(
		"OutlineGenerator",
		swarm.EventStart,
		func(ctx *swarm.Context, event swarm.Event) (swarm.Event, error) {
			return handleStartEvent(ctx, event, client)
		},
		swarm.StepConfig{
			RetryPolicy: &swarm.RetryPolicy{
				MaxRetries:      3,
				InitialInterval: time.Second,
				MaxInterval:     10 * time.Second,
				Multiplier:      2.0,
			},
		},
	)

	// Add step to write chapters in parallel
	parallelStep := swarm.NewStep(
		"ChapterParallelizer",
		EventOutline,
		handleOutlineEvent,
		swarm.StepConfig{},
	)

	// Add step to write individual chapters
	writeChapterStep := swarm.NewStep(
		"ChapterWriter",
		swarm.EventType("WriteChapter"),
		func(ctx *swarm.Context, event swarm.Event) (swarm.Event, error) {
			return handleWriteChapter(ctx, event, client)
		},
		swarm.StepConfig{
			MaxParallel: 2,
		},
	)

	// Add step to collect chapters and create final novel
	finalizeStep := swarm.NewStep(
		"NovelFinalizer",
		swarm.EventParallelResult,
		handleOutlineEventResult,
		swarm.StepConfig{},
	)

	// Add steps to workflow
	for _, step := range []swarm.Step{outlineStep, parallelStep, writeChapterStep, finalizeStep} {
		if err := workflow.AddStep(step); err != nil {
			fmt.Printf("Failed to add step %s: %v\n", step.Name(), err)
			os.Exit(1)
		}
	}

	// Run workflow
	ctx := context.Background()
	inputs := map[string]interface{}{
		"topic": "a time traveler who accidentally changes history",
	}
	handler, err := workflow.Run(ctx, inputs)
	if err != nil {
		fmt.Printf("Failed to start workflow: %v\n", err)
		return
	}

	// Stream events for real-time progress tracking
	go func() {
		trackSteps(handler)
	}()

	// Wait for workflow finish
	result, err := handler.Wait()
	if err != nil {
		fmt.Printf("Workflow failed: %v\n", err)
		return
	}

	// Output the final novel
	if result != nil {
		data := result.(map[string]interface{})
		fmt.Printf("\nNovel about: %s\n\n", data["topic"])

		chapters := data["chapters"].(map[string]string)
		chapterTitles := data["chapter_titles"].([]string)
		for _, title := range chapterTitles {
			if content, ok := chapters[title]; ok {
				fmt.Printf("%s\n\n", content)
			}
		}
	}
}

func trackSteps(handler *swarm.WorkflowHandler) {
	for event := range handler.Stream() {
		switch event.Type() {
		case EventOutline:
			if outline, ok := event.(*OutlineEvent); ok {
				fmt.Printf("\nGenerated outline for novel about %s:\n", outline.Topic)
				for i, chapter := range outline.Chapters {
					fmt.Printf("Chapter %d: %s\n", i+1, chapter)
				}
			}
		case EventChapter:
			if chapter, ok := event.(*ChapterEvent); ok {
				fmt.Printf("\nCompleted chapter: %s\n", chapter.Title)
			}
		case swarm.EventParallelResult:
			if result, ok := event.(*swarm.ParallelResultEvent); ok {
				successful, failed, duration := result.GetStats()
				fmt.Printf("\nParallel execution stats:\n- Successful: %d\n- Failed: %d\n- Duration: %s\n",
					successful, failed, duration)
			}
		}
	}
}
