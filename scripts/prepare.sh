#!/bin/bash

# Установка зависимостей
go mod init project
go get github.com/lib/pq


# Создание пользователя и базы данных
psql -U postgres -c "CREATE USER validator WITH PASSWORD 'val1dat0r';"
psql -U postgres -c "CREATE DATABASE project-sem-1;"
psql -U postgres -d project-sem-1 -c "
CREATE TABLE IF NOT EXISTS prices (
    id TEXT,
    name TEXT,
    category TEXT,
    price INTEGER,
    create_date DATE
);
"