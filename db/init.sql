CREATE TABLE animals (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    animal_type VARCHAR(50) NOT NULL,
    age INTEGER,
    arrival_date TIMESTAMP DEFAULT NOW(),
    health_status VARCHAR(100)
);

-- Тестовые данные (опционально)
INSERT INTO animals (name, animal_type, age, health_status) VALUES
('Барсик', 'кот', 3, 'здоров'),
('Шарик', 'собака', 5, 'на лечении');