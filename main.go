package main

import (
    "archive/zip"
    "bytes"
    "database/sql"
    "encoding/csv"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net/http"
    "strconv"
)

const (
    host     = "localhost"
    port     = 5432
    user     = "validator"
    password = "val1dat0r"
    dbname   = "project-sem-1"
)

func initDatabase(db *sql.DB) error {
    // SQL-запрос для создания таблицы prices, если она не существует
    createTableQuery := `
        CREATE TABLE IF NOT EXISTS prices (
            id SERIAL PRIMARY KEY,
            name VARCHAR(255) NOT NULL,
            category VARCHAR(255) NOT NULL,
            price INTEGER NOT NULL,
            create_date DATE NOT NULL
        );
    `

    // Выполнение запроса
    _, err := db.Exec(createTableQuery)
    if err != nil {
        return fmt.Errorf("error creating table: %v", err)
    }

    log.Println("Таблица 'prices' успешно создана (если не существовала)")
    return nil
}

func main() {
    router := http.NewServeMux()

    // Единый обработчик для /api/v0/prices
    router.HandleFunc("/api/v0/prices", func(w http.ResponseWriter, r *http.Request) {
        switch r.Method {
        case http.MethodPost:
            // Обработка POST-запроса
            file, _, err := r.FormFile("file")
            if err != nil {
                http.Error(w, "Error reading file", http.StatusBadRequest)
                return
            }
            defer file.Close()

            buf := new(bytes.Buffer)
            _, err = buf.ReadFrom(file)
            if err != nil {
                http.Error(w, "Error reading file content", http.StatusBadRequest)
                return
            }

            reader, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
            if err != nil {
                http.Error(w, "Error unzipping file", http.StatusBadRequest)
                return
            }

            var totalItems, totalCategories, totalPrice int
            categorySet := make(map[string]struct{})

            for _, f := range reader.File {
                if f.Name == "data.csv" {
                    csvFile, err := f.Open()
                    if err != nil {
                        http.Error(w, "Error opening CSV file", http.StatusInternalServerError)
                        return
                    }
                    defer csvFile.Close()

                    reader := csv.NewReader(csvFile)
                    rows, err := reader.ReadAll()
                    if err != nil {
                        http.Error(w, "Error reading CSV", http.StatusInternalServerError)
                        return
                    }
                    
                    db, err := sql.Open("postgres", fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", host, port, user, password, dbname))
                    if err != nil {
                        log.Printf("Ошибка при подключении к базе данных: %v", err)
                        http.Error(w, "Database connection error", http.StatusInternalServerError)
                        return
                    }
                    defer db.Close()
                    
                    // Инициализация базы данных (создание таблицы, если она не существует)
                    if err := initDatabase(db); err != nil {
                        log.Printf("Ошибка инициализации базы данных: %v", err)
                        http.Error(w, "Database initialization error", http.StatusInternalServerError)
                        return
                    }
                    
                    log.Println("База данных успешно инициализирована")

                    tx, err := db.Begin()
                    if err != nil {
                        http.Error(w, "Transaction error", http.StatusInternalServerError)
                        return
                    }

                    stmt, err := tx.Prepare(`INSERT INTO prices (id, name, category, price, create_date) VALUES ($1, $2, $3, $4, $5)`)
                    if err != nil {
                        http.Error(w, "SQL preparation error", http.StatusInternalServerError)
                        return
                    }

                    for _, row := range rows[1:] {
                        id := row[0]
                        name := row[1]
                        category := row[2]
                        priceStr := row[3]
                        createDate := row[4]

                        price, err := strconv.Atoi(priceStr)
                        if err != nil {
                            continue
                        }

                        _, err = stmt.Exec(id, name, category, price, createDate)
                        if err != nil {
                            tx.Rollback()
                            http.Error(w, "Error inserting data", http.StatusInternalServerError)
                            return
                        }

                        totalItems++
                        totalPrice += price
                        categorySet[category] = struct{}{}
                    }

                    err = tx.Commit()
                    if err != nil {
                        http.Error(w, "Transaction commit error", http.StatusInternalServerError)
                        return
                    }

                    totalCategories = len(categorySet)
                }
            }

            response := map[string]int{
                "total_items":      totalItems,
                "total_categories": totalCategories,
                "total_price":      totalPrice,
            }

            w.Header().Set("Content-Type", "application/json")
            json.NewEncoder(w).Encode(response)

        case http.MethodGet:
            // Обработка GET-запроса
            db, err := sql.Open("postgres", fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", host, port, user, password, dbname))
            if err != nil {
                log.Printf("Ошибка при подключении к базе данных: %v", err)
                http.Error(w, "Database connection error", http.StatusInternalServerError)
                return
            }
            defer db.Close()
            
            log.Println("База данных успешно инициализирована")

            rows, err := db.Query("SELECT id, name, category, price, create_date FROM prices")
            if err != nil {
                http.Error(w, "Error querying database", http.StatusInternalServerError)
                return
            }
            defer rows.Close()

            var records [][]string
            for rows.Next() {
                var id, name, category, createDate string
                var price int
                err := rows.Scan(&id, &name, &category, &price, &createDate)
                if err != nil {
                    http.Error(w, "Error scanning rows", http.StatusInternalServerError)
                    return
                    
                }
                rows.Close()
                records = append(records, []string{id, name, category, strconv.Itoa(price), createDate})
            }

            // Создаем CSV файл
            csvData := &bytes.Buffer{}
            writer := csv.NewWriter(csvData)
            writer.Write([]string{"id", "name", "category", "price", "create_date"})
            for _, record := range records {
                writer.Write(record)
            }
            writer.Flush()

            // Создаем ZIP архив
            zipBuffer := new(bytes.Buffer)
            zipWriter := zip.NewWriter(zipBuffer)
            fileWriter, _ := zipWriter.Create("data.csv")
            io.Copy(fileWriter, csvData)
            zipWriter.Close()

            // Возвращаем ZIP архив
            w.Header().Set("Content-Type", "application/zip")
            w.Header().Set("Content-Disposition", "attachment; filename=data.zip")
            w.Write(zipBuffer.Bytes())

        default:
            // Ответ для неподдерживаемых методов
            http.Error(w, "Invalid method", http.StatusMethodNotAllowed)
        }
    })

    fmt.Println("Server started on :8080")
    http.ListenAndServe(":8080", router)
}

// 17 try