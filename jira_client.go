package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type JiraClient struct {
	BaseURL string

	HTTPClient *http.Client
}

type jiraClientTransportWrapper struct {
	agent     string
	token     string
	cookies   []*http.Cookie
	transport http.RoundTripper
}

func (t *jiraClientTransportWrapper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Accept", "application/json; charset=UTF-8")
	if t.token != "" {
		req.Header.Set("Authorization", "Bearer "+t.token)
	}
	if t.agent != "" {
		req.Header.Set("User-Agent", t.agent)
	}
	for _, cookie := range t.cookies {
		req.AddCookie(cookie)
	}
	if req.Method == "POST" {
		req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	}
	return t.transport.RoundTrip(req)
}

type JiraIssue struct {
	ID  string `json:"id"`
	Key string `json:"key"`
}

type StructureRowAttributes struct {
	Duration     time.Duration
	ManualStart  time.Time
	ManualFinish time.Time
	Start        time.Time
	Finish       time.Time
	Signature    int64
	Version      int
}

type GanttMeta struct {
	Calendar    Calendar
	StartDateId int
	ZoneId      string // todo: использовать
}

func NewJiraClient(cfg ClientConfig) *JiraClient {
	// Создаем клиента
	transportWrapper := &jiraClientTransportWrapper{
		agent:     cfg.UserName,
		token:     cfg.Token,
		transport: http.DefaultTransport,
	}
	for _, c := range cfg.Cookies {
		transportWrapper.cookies = append(transportWrapper.cookies, &http.Cookie{
			Name:  c.Name,
			Value: c.Value,
		})
	}
	client := &JiraClient{
		BaseURL: cfg.URL,
		HTTPClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: transportWrapper,
		},
	}
	return client
}

// --- Методы ---

func (c *JiraClient) GetIssues(jql string) ([]JiraIssue, error) {
	url := fmt.Sprintf("%s/rest/api/latest/search?jql=%s&maxResults=1000&fields=summary", c.BaseURL, url.QueryEscape(jql))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка выполнения запроса: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ошибка ответа: %d", resp.StatusCode)
	}

	var result struct {
		Issues []JiraIssue `json:"issues"`
	}

	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	return result.Issues, nil
}

func (c *JiraClient) GetForestMapping(structureID int) (map[string]string, error) {
	forestSpec := fmt.Sprintf(`{"structureId":%d}`, structureID)
	forestURL := fmt.Sprintf("%s/rest/structure/2.0/forest/latest?s=%s", c.BaseURL, url.QueryEscape(forestSpec))

	req, err := http.NewRequest("GET", forestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка отправки запроса: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ошибка ответа: %d", resp.StatusCode)
	}

	var forest struct {
		Spec struct {
			StructureId int `json:"structureId"`
		} `json:"spec"`
		Formula   string            `json:"formula"`
		ItemTypes map[string]string `json:"itemTypes"`
	}
	err = json.NewDecoder(resp.Body).Decode(&forest)
	if err != nil {
		return nil, fmt.Errorf("ошибка парсинга Forest: %w", err)
	}

	issueIDToRowID := make(map[string]string)

	// Парсим formula
	formulaItems := strings.Split(forest.Formula, ",")
	for _, item := range formulaItems {
		parts := strings.Split(item, ":")
		if len(parts) < 3 {
			continue
		}
		rowID := parts[0]
		itemIdentity := parts[2]

		// Только для задач (нет / в itemIdentity)
		if strings.Contains(itemIdentity, "/") {
			continue
		}

		issueIDToRowID[itemIdentity] = rowID
	}

	return issueIDToRowID, nil
}

func (c *JiraClient) GetRowAttributes(structureID int, rowID int) (*StructureRowAttributes, error) {
	url := fmt.Sprintf("%s/rest/structure/2.0/attribute/subscription?valuesUpdate=true&valuesTimeout=500", c.BaseURL)

	requestBody := map[string]interface{}{
		"forestSpec": map[string]interface{}{
			"structureId": structureID,
		},
		"rows": []int{rowID},
		"attributes": []map[string]string{
			{"id": "gantt.duration", "format": "text"},
			{"id": "gantt.manualStart", "format": "text"},
			{"id": "gantt.manualFinish", "format": "text"},
			{"id": "gantt.start", "format": "text"},
			{"id": "gantt.finish", "format": "text"},
		},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("ошибка сериализации тела запроса: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ошибка выполнения запроса: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ошибка ответа: %d", resp.StatusCode)
	}

	var rawResponse struct {
		ValuesUpdate struct {
			Version struct {
				Signature int64 `json:"signature"`
				Version   int   `json:"version"`
			} `json:"version"`
			Data []struct {
				Attribute struct {
					ID string `json:"id"`
				} `json:"attribute"`
				Values map[string]string `json:"values"`
			} `json:"data"`
		} `json:"valuesUpdate"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rawResponse); err != nil {
		return nil, fmt.Errorf("ошибка парсинга ответа: %w", err)
	}

	attributes := StructureRowAttributes{
		Signature: rawResponse.ValuesUpdate.Version.Signature,
		Version:   rawResponse.ValuesUpdate.Version.Version,
	}

	// Временные переменные для текстовых значений
	var durationStr, manulStartStr, manualFinishStr, startStr, finishStr string

	for _, data := range rawResponse.ValuesUpdate.Data {
		for row, value := range data.Values {
			if row != fmt.Sprintf("%d", rowID) {
				continue
			}
			switch data.Attribute.ID {
			case "gantt.duration":
				durationStr = value
			case "gantt.manualStart":
				manulStartStr = value
			case "gantt.manualFinish":
				manualFinishStr = value
			case "gantt.start":
				startStr = value
			case "gantt.finish":
				finishStr = value
			}
		}
	}

	// Парсим даты
	layout := "02/Jan/06 15:04 PM" // формат даты из примера ответа
	if manulStartStr != "" {
		t, err := time.Parse(layout, manulStartStr)
		if err != nil {
			return nil, fmt.Errorf("ошибка парсинга manualStart: %w", err)
		}
		attributes.ManualStart = t
	}
	if manualFinishStr != "" {
		t, err := time.Parse(layout, manualFinishStr)
		if err != nil {
			return nil, fmt.Errorf("ошибка парсинга manualFinishStr: %w", err)
		}
		attributes.ManualFinish = t
	}
	if startStr != "" {
		t, err := time.Parse(layout, startStr)
		if err != nil {
			return nil, fmt.Errorf("ошибка парсинга start: %w", err)
		}
		attributes.Start = t
	}
	if finishStr != "" {
		t, err := time.Parse(layout, finishStr)
		if err != nil {
			return nil, fmt.Errorf("ошибка парсинга finishStr: %w", err)
		}
		attributes.Finish = t
	}

	// Парсим duration
	if durationStr != "" {
		dur, err := parseGanttDuration(durationStr)
		if err != nil {
			return nil, fmt.Errorf("ошибка парсинга duration: %w", err)
		}
		attributes.Duration = dur
	}

	return &attributes, nil
}

func (c *JiraClient) UpdateLevelingDelay(ganttId, rowId int, delay time.Duration, versionSignature int64, VersionNumber int) error {
	url := fmt.Sprintf("%s/rest/structure-gantt/1.0/chart/%d/actions", c.BaseURL, ganttId)

	// Создание тела запроса с inline-структурами
	payload := struct {
		ZoneID  string `json:"zoneId"`
		Changes []struct {
			RowID int    `json:"rowId"`
			Delay int64  `json:"delay"`
			Type  string `json:"type"`
		} `json:"changes"`
		Version struct {
			Signature int64 `json:"signature"`
			Version   int   `json:"version"`
		} `json:"version"`
	}{
		ZoneID: "Etc/UTC",
		Changes: []struct {
			RowID int    `json:"rowId"`
			Delay int64  `json:"delay"`
			Type  string `json:"type"`
		}{
			{
				RowID: rowId,
				Delay: int64(delay / time.Millisecond),
				Type:  "changeLevelingDelay",
			},
		},
		Version: struct {
			Signature int64 `json:"signature"`
			Version   int   `json:"version"`
		}{
			Signature: versionSignature,
			Version:   VersionNumber,
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("ошибка сериализации JSON: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("ошибка создания запроса: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка выполнения запроса: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ошибка ответа: %d", resp.StatusCode)
	}

	return nil
}

func (c *JiraClient) GetGanttMeta(structureID int) (*GanttMeta, error) {
	url := fmt.Sprintf("%s/rest/structure/2.0/poll", c.BaseURL)

	payload := map[string]interface{}{
		"extensionRequests": map[string]interface{}{
			"com.almworks.structure.gantt:gantt-extension": map[string]interface{}{
				"structureId": structureID,
				"ganttId":     structureID,
			},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ошибка ответа: %d", resp.StatusCode)
	}

	type JsonTimeRange struct {
		StartTimeId  int `json:"startTimeId"`
		FinishTimeId int `json:"finishTimeId"`
	}
	type JsonCalendar struct {
		ID       int    `json:"id"`
		Name     string `json:"name"`
		WeekDays []struct {
			TimeRanges []JsonTimeRange `json:"timeRanges"`
		} `json:"weekDays"`
		CustomDays []struct {
			DateId   int `json:"dateId"`
			Schedule struct {
				TimeRanges []JsonTimeRange `json:"timeRanges"`
			} `json:"schedule"`
		} `json:"customDays"`
	}

	var result struct {
		ExtensionData map[string]struct {
			CalendarID int    `json:"calendarId"`
			ZoneId     string `json:"zoneId"`
			Gantt      struct {
				StartDateId int `json:"startDateId"`
			} `json:"gantt"`
			Calendars []JsonCalendar `json:"calendars"`
		} `json:"extensionData"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	ext := result.ExtensionData["com.almworks.structure.gantt:gantt-extension"]

	var foundRawCal *JsonCalendar
	for _, cal := range ext.Calendars {
		if cal.ID == ext.CalendarID {
			foundRawCal = &cal
			break
		}
	}
	if foundRawCal == nil {
		return nil, fmt.Errorf("calendar with ID %d not found", ext.CalendarID)
	}

	convertTimeRanges := func(trs []JsonTimeRange) DaySchedule {
		var ranges []TimeRange
		var total time.Duration
		for _, tr := range trs {
			start := parseTimeId(tr.StartTimeId)
			finish := parseTimeId(tr.FinishTimeId)
			ranges = append(ranges, TimeRange{
				StartTimeId:  tr.StartTimeId,
				FinishTimeId: tr.FinishTimeId,
			})
			total += finish - start
		}
		return DaySchedule{
			TimeRanges: ranges,
			Duration:   total,
		}
	}

	// Преобразование в структуру Calendar
	var cal Calendar
	cal.ID = foundRawCal.ID
	cal.Name = foundRawCal.Name
	cal.WeekDays = make([]DaySchedule, len(foundRawCal.WeekDays))
	for i, wd := range foundRawCal.WeekDays {
		cal.WeekDays[i] = convertTimeRanges(wd.TimeRanges)
	}
	cal.CustomDays = make(map[int]DaySchedule)
	for _, cd := range foundRawCal.CustomDays {
		cal.CustomDays[cd.DateId] = convertTimeRanges(cd.Schedule.TimeRanges)
	}

	return &GanttMeta{
		Calendar:    cal,
		StartDateId: ext.Gantt.StartDateId,
		ZoneId:      ext.ZoneId,
	}, nil
}
