package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"
)

func main() {
	log.SetOutput(os.Stdout)

	config := flag.String("c", "config.yml", "Путь к конфигурационному файлу YAML (если не указано — config.yml)")
	structure := flag.String("s", "", "Название секции из 'structures' для выполнения (если не указано — выполняются все)")

	flag.Parse()

	if config == nil || *config == "" {
		fmt.Println("Usage:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	cfg, err := loadConfig(*config)
	if err != nil {
		log.Fatalf("Не удалось загрузить кнфигурационный файл: %v", err)
	}

	// Создаем клиента
	client := NewJiraClient(cfg.Client)
	if structure != nil && *structure != "" {
		if _, ok := cfg.Structures[*structure]; !ok {
			log.Fatalf("В спикке структур нет настроек для '%s' в конфигурационном файле", *structure)
		}
		log.Printf("Выставляем задержки выравнивания для структуры '%s'\n", *structure)
		err = calculateLeveling(client, cfg.Structures[*structure])
		if err != nil {
			log.Fatalf("Не удалось выставить задержки для структуры '%s': %v", *structure, err)
		}
		return
	}
	for structureName, structureCfg := range cfg.Structures {
		log.Printf("Выставляем задержки выравнивания для структуры '%s'\n", structureName)
		err = calculateLeveling(client, structureCfg)
		if err != nil {
			log.Fatalf("Не удалось выставить задержки выравнивания для структуры '%s': %v", structureName, err)
		}
	}
}

func calculateLeveling(client *JiraClient, structure StructureConfig) error {
	log.Printf("Получаем информацию о Gantt-диограмме для структуры %d\n", structure.ID)
	gantt, err := client.GetGanttMeta(structure.ID)
	if err != nil {
		return fmt.Errorf("ошибка получения информации о Gantt-диограмме: %v", err)
	}

	log.Printf("Получаем соответсвие issueID к rowID в структуре %d\n", structure.ID)
	issueIDToRowID, err := client.GetForestMapping(structure.ID)
	if err != nil {
		return fmt.Errorf("ошибка получения соответсвия issueID к rowID: %v", err)
	}

	log.Printf("Получаем список задач по JQL: '%s'\n", structure.JQL)
	issues, err := client.GetIssues(structure.JQL)
	if err != nil {
		return fmt.Errorf("ошибка получения списка зададач: %w", err)
	}

	// Создаем слоты по количеству параллельных проектов
	// Каждый слот будет хранить задержку от начала проекта в кол-ве рабочих часов
	var slots Slots
	todayId := structure.StartDateID
	if todayId <= 0 {
		todayId = dateIdFromTime(time.Now())
	}
	if gantt.StartDateId >= todayId {
		// если дата начала проекта совпадает с текущей датой или в будущем, то задержки от начала проекта во всех слотах нет
		slots = NewSlots(structure.ParallelProjects, 0)
	} else {
		// Если дата начала в прошлом, выставляем в каждом слоте задержку равную кол-ву рабочих часов между датой начала проекта и текущей
		// Выравнивание задач начнется с текущей даты
		slots = NewSlots(structure.ParallelProjects, gantt.Calendar.GetWorkingDurationBetween(gantt.StartDateId, todayId))
	}

	for _, issue := range issues {
		log.Printf("Рассчитываем задержку выравнивания для задачи %s\n", issue.Key)
		rowID, ok := issueIDToRowID[issue.ID]
		if !ok {
			log.Printf("[WARNING] Задачи %s (%s) нет в структуре %d. Задача будет пропущена.\n", issue.Key, issue.ID, structure.ID)
			continue
		}

		rowIDInt, err := parseInt(rowID)
		if err != nil {
			return fmt.Errorf("ошибка преобразования rowID в число: %w", err)
		}

		log.Printf("Получаем текущие атрибуты из Gantt для задачи %s\n", issue.Key)
		attributes, err := client.GetRowAttributes(structure.ID, rowIDInt)
		if err != nil {
			return fmt.Errorf("ошибка получения атрибутов: %v", err)
		}

		// Определяем задержку для задачи относительно начала проекта
		var levelingDelay time.Duration

		// Если для задачи в ручную выставлены дата начала или окончания, выставление задержки не нужно.
		// Выбираем наименьший слот и выставляем в него дату смещение рассчитанное
		// TODO: обработать корнер кейсы. Тут сделано допущение, что JQL возвращает задачи отсортированные по дате завершения
		if !attributes.ManualStart.IsZero() || !attributes.ManualFinish.IsZero() {
			slot, _ := slots.FindSlot()
			slots.SetDelay(
				slot,
				gantt.Calendar.GetWorkingDurationBetween(gantt.StartDateId, dateIdFromTime(attributes.Start))+attributes.Duration,
			)
		} else {
			levelingDelay, _ = slots.GetLevelingDelayAndAdd(attributes.Duration)
		}

		log.Printf("Выставляем задержку выравнивания %s для задачи %s\n", levelingDelay, issue.Key)
		err = client.UpdateLevelingDelay(structure.ID, rowIDInt, levelingDelay, attributes.Signature, attributes.Version)
		if err != nil {
			log.Fatalf("ошибка обновления задержки выравнивания: %v", err)
		}
	}

	return nil
}
