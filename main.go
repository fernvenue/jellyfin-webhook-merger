package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	flag "github.com/spf13/pflag"
)

var version = "dev"

type WebhookRequest struct {
	SeriesId      string `json:"SeriesId"`
	SeriesName    string `json:"SeriesName"`
	SeasonNumber  int    `json:"SeasonNumber"`
	EpisodeNumber int    `json:"EpisodeNumber"`
	EpisodeName   string `json:"EpisodeName"`
}

type QueueKey struct {
	SeriesId     string
	SeasonNumber int
}

type Episode struct {
	EpisodeNumber int
	EpisodeName   string
}

type QueueValue struct {
	SeriesName string
	Episodes   []Episode
}

type Config struct {
	ListenAddress    string
	ListenPort       int
	WaitSecond       int
	RetryCount       int
	TextContent      string
	TextKey          string
	EpisodeFormat    string
	TargetURL        string
	AdditionalParams string
	ContentHeader    string
}

var (
	config      Config
	queue       = make(map[QueueKey]QueueValue)
	mu          sync.Mutex
	showVersion bool
	showHelp    bool
)

func envStr(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultVal
}

func main() {
	flag.StringVarP(&config.ListenAddress, "listen-address", "a", envStr("LISTEN_ADDRESS", "::1"), "Bind address.")
	flag.IntVarP(&config.ListenPort, "listen-port", "p", envInt("LISTEN_PORT", 8520), "Bind port.")
	flag.IntVarP(&config.WaitSecond, "wait-second", "w", envInt("WAIT_SECOND", 300), "Wait time in seconds before merging the notifications.")
	flag.IntVarP(&config.RetryCount, "retry-count", "r", envInt("RETRY_COUNT", 3), "Number of times to retry sending the notification if the target URL does not return a 2xx response.")
	flag.StringVarP(&config.TextKey, "text-key", "k", envStr("TEXT_KEY", "text"), "Key used for the notification text in the JSON payload.")
	flag.StringVarP(&config.TextContent, "text-content", "t", envStr("TEXT_CONTENT", "📺 <b>Episode update reminder:</b> <b>{{.SeriesName}}</b> <b>Season {{.SeasonNumber}}</b>\n"), "Template for the notification text.")
	flag.StringVarP(&config.EpisodeFormat, "episode-format", "e", envStr("EPISODE_FORMAT", "\nEpisode {{.EpisodeNumber}} {{.EpisodeName}}"), "Format for each episode's notification line.")
	flag.StringVarP(&config.TargetURL, "target-url", "u", envStr("TARGET_URL", ""), "Target URL to send the notification to.")
	flag.StringVarP(&config.AdditionalParams, "additional-params", "d", envStr("ADDITIONAL_PARAMS", "{}"), "Additional parameters in JSON format, supports variables like '{{.SeriesId}}'.")
	flag.StringVarP(&config.ContentHeader, "content-header", "c", envStr("CONTENT_HEADER", "text"), "Content type hint used when building the outgoing request.")
	flag.BoolVarP(&showVersion, "version", "v", false, "Print version and exit.")
	flag.BoolVarP(&showHelp, "help", "h", false, "Print help and exit.")
	flag.Parse()

	if showHelp {
		flag.Usage()
		os.Exit(0)
	}

	if showVersion {
		fmt.Println(version)
		return
	}

	if config.TargetURL == "" {
		log.Fatal("Error: target-url is required")
	}

	if err := validateJSON(config.AdditionalParams); err != nil {
		log.Fatalf("Invalid JSON in --additional-params: %v", err)
	}

	http.HandleFunc("/", handleWebhook)
	http.HandleFunc("/200", helloWorld)
	address := fmt.Sprintf("[%s]:%d", config.ListenAddress, config.ListenPort)
	log.Printf("Server started at %s", address)
	log.Fatal(http.ListenAndServe(address, nil))
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req WebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("Error decoding request: %v", err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	log.Printf("Received request: %v", req)

	key := QueueKey{SeriesId: req.SeriesId, SeasonNumber: req.SeasonNumber}
	mu.Lock()
	value, exists := queue[key]
	if !exists {
		value = QueueValue{SeriesName: req.SeriesName, Episodes: []Episode{}}
	}
	value.Episodes = append(value.Episodes, Episode{
		EpisodeNumber: req.EpisodeNumber,
		EpisodeName:   req.EpisodeName,
	})
	queue[key] = value
	mu.Unlock()

	if !exists {
		log.Printf("Starting to process queue for SeriesId: %s, SeasonNumber: %d", req.SeriesId, req.SeasonNumber)
		go processQueue(key)
	}
	w.WriteHeader(http.StatusOK)
}

func processQueue(key QueueKey) {
	time.Sleep(time.Duration(config.WaitSecond) * time.Second)

	mu.Lock()
	value := queue[key]
	delete(queue, key)
	mu.Unlock()

	log.Printf("Processing queue for SeriesId: %s, SeasonNumber: %d", key.SeriesId, key.SeasonNumber)

	sort.Slice(value.Episodes, func(i, j int) bool {
		return value.Episodes[i].EpisodeNumber < value.Episodes[j].EpisodeNumber
	})

	text, err := buildText(value.SeriesName, key.SeasonNumber, value.Episodes)
	if err != nil {
		log.Printf("Error building text: %v", err)
		return
	}

	params := map[string]interface{}{}
	if err := json.Unmarshal([]byte(config.AdditionalParams), &params); err != nil {
		log.Printf("Error unmarshalling additional params: %v", err)
		return
	}

	tmpl, err := template.New("additionalParams").Parse(config.AdditionalParams)
	if err != nil {
		log.Printf("Error parsing additional params template: %v", err)
		return
	}

	var paramBuf strings.Builder
	err = tmpl.Execute(&paramBuf, struct {
		SeriesId string
	}{
		SeriesId: key.SeriesId,
	})
	if err != nil {
		log.Printf("Error executing additional params template: %v", err)
		return
	}

	finalParams := paramBuf.String()
	if err := json.Unmarshal([]byte(finalParams), &params); err != nil {
		log.Printf("Error unmarshalling final params: %v", err)
		return
	}

	params[config.TextKey] = text

	body, _ := json.Marshal(params)

	var sendErr error
	for attempt := 0; attempt <= config.RetryCount; attempt++ {
		if attempt > 0 {
			log.Printf("Retry attempt %d/%d, waiting %d seconds", attempt, config.RetryCount, config.WaitSecond)
			time.Sleep(time.Duration(config.WaitSecond) * time.Second)
		}

		log.Printf("Sending request to target URL: %s", config.TargetURL)
		log.Printf("Request body: %s", string(body))

		resp, err := http.Post(config.TargetURL, "application/json", bytes.NewReader(body))
		if err != nil {
			log.Printf("Error sending request to target URL (attempt %d): %v", attempt+1, err)
			sendErr = err
			continue
		}

		respBody := new(bytes.Buffer)
		respBody.ReadFrom(resp.Body)
		resp.Body.Close()

		log.Printf("Response status: %s", resp.Status)
		log.Printf("Response body: %s", respBody.String())

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			sendErr = nil
			break
		}

		sendErr = fmt.Errorf("target returned status: %s", resp.Status)
		log.Printf("Non-2xx response (attempt %d): %s", attempt+1, resp.Status)
	}

	if sendErr != nil {
		log.Printf("All %d retry attempts failed: %v", config.RetryCount, sendErr)
	}
}

func buildText(seriesName string, seasonNumber int, episodes []Episode) (string, error) {
	textTmpl, err := template.New("text").Parse(config.TextContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse text template: %v", err)
	}

	episodeTmpl, err := template.New("episode").Parse(config.EpisodeFormat)
	if err != nil {
		return "", fmt.Errorf("failed to parse episode template: %v", err)
	}

	textData := struct {
		SeriesName   string
		SeasonNumber int
	}{
		SeriesName:   seriesName,
		SeasonNumber: seasonNumber,
	}

	var textBuf strings.Builder
	if err := textTmpl.Execute(&textBuf, textData); err != nil {
		return "", fmt.Errorf("failed to execute text template: %v", err)
	}

	textWithBr := textBuf.String()

	var episodeTextBuf strings.Builder
	for _, ep := range episodes {
		epData := struct {
			EpisodeNumber int
			EpisodeName   string
		}{
			EpisodeNumber: ep.EpisodeNumber,
			EpisodeName:   ep.EpisodeName,
		}
		if err := episodeTmpl.Execute(&episodeTextBuf, epData); err != nil {
			return "", fmt.Errorf("failed to execute episode template: %v", err)
		}
	}

	finalText := textWithBr + episodeTextBuf.String()

	finalText = strings.ReplaceAll(finalText, "\\n", "\n")

	return finalText, nil
}

func validateJSON(input string) error {
	var js map[string]interface{}
	return json.Unmarshal([]byte(input), &js)
}

func helloWorld(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Hello, World!"))
}
