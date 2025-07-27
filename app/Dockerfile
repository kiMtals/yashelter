# Используем лёгкий образ Python
FROM python:3.9-slim

# Рабочая директория
WORKDIR /app

# Копируем зависимости
COPY requirements.txt .

# Устанавливаем зависимости
RUN pip install --no-cache-dir -r requirements.txt

# Копируем код приложения
COPY . .

# Открываем порт 8000
EXPOSE 8000

# Запускаем приложение
CMD ["gunicorn", "--bind", "0.0.0.0:8000", "app:app"]