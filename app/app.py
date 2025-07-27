import random
from flask import Flask, request, jsonify, Response
import psycopg2
import time
from datetime import datetime
from prometheus_client import Counter, Histogram, generate_latest, CONTENT_TYPE_LATEST
from flasgger import Swagger
from flask import render_template_string

app = Flask(__name__)
app.config["SWAGGER"] = {
    "title": "Shelter API",
    "uiversion": 3,
    "specs_route": "/apidocs"
}
swagger = Swagger(app)

REQUEST_COUNT = Counter('http_requests_total', 'Total HTTP Requests', ['method', 'endpoint'])
REQUEST_LATENCY = Histogram('http_request_duration_seconds', 'HTTP Request Latency', ['endpoint'])

def get_db():
    try:
        return psycopg2.connect(
            dbname="shelter_db",
            user="postgres",
            password="password",
            host="db"
        )
    except Exception as e:
        app.logger.error(f"Database connection error: {e}")
        raise

@app.route('/')
def home():
    base_url = request.host_url.rstrip('/')
    return render_template_string("""
    <!DOCTYPE html>
    <html>
    <head>
        <meta charset="UTF-8">
        <title>Яндекс.Приют</title>
        <style>
            body { font-family: Arial, sans-serif; background: #f4f4f4; padding: 2em; color: #333; }
            h1 { color: #8A2BE2; }
            ul { list-style: none; padding-left: 0; }
            li { margin-bottom: 0.5em; }
            a { text-decoration: none; color: #007bff; }
            a:hover { text-decoration: underline; }
        </style>
    </head>
    <body>
        <h1>🐶 Добро пожаловать в Яндекс.Приют!</h1>
        <p>Вы можете:</p>
        <ul>
            <li><a href="{{ base_url }}/docs">📘 Документация</a></li>
            <li><a href="{{ base_url }}/main">🐾 UI: Животные</a></li>
            <li><a href="{{ base_url }}/animals">🧾 JSON: Список животных</a></li>
            <li><a href="{{ base_url }}/metrics">📈 Метрики Prometheus</a></li>
            <li><a href="{{ base_url }}/apidocs">🧪 Swagger UI</a></li>
            <li><a href="{{ base_url.replace(':8000', ':9090') }}">🔎 Prometheus</a></li>
            <li><a href="{{ base_url.replace(':8000', ':9093') }}">🚨 Alertmanager</a></li>
            <li><a href="{{ base_url.replace(':8000', ':3000') }}">📊 Grafana</a></li>
            <li><a href="{{ base_url.replace(':8000', ':3006') }}">⚙️ Нагрузочное тестирование</a></li>
        </ul>
    </body>
    </html>
    """, base_url=base_url)


@app.route('/docs')
def docs():
    base_url = request.host_url.rstrip('/')
    return f"""
    <h1>📘 Документация API</h1>
    <ul>
        <li><b>GET</b> <a href="{base_url}/">/</a> — Главная страница</li>
        <li><b>GET</b> <a href="{base_url}/animals">/animals</a> — Список животных</li>
        <li><b>POST</b> <code>/api/animals</code> — Добавить животное</li>
        <li><b>GET</b> <a href="{base_url}/slow">/slow</a> — Медленный эндпоинт (для алертов)</li>
        <li><b>GET</b> <a href="{base_url}/metrics">/metrics</a> — Метрики Prometheus</li>
        <li><b>UI</b> <a href="{base_url}/apidocs">Swagger UI</a> — Интерактивная документация</li>
    </ul>
    <hr>
    <h3>🔗 Ссылки на сервисы</h3>
    <ul>
        <li><a href="{base_url.replace(':8000', ':9090')}">Prometheus</a></li>
        <li><a href="{base_url.replace(':8000', ':9093')}">Alertmanager</a></li>
        <li><a href="{base_url.replace(':8000', ':3000')}">Grafana</a></li>
        <li><a href="{base_url.replace(':8000', ':3006')}">Load tester</a></li>
    </ul>
    """

@app.route('/animals', methods=['GET'])
def list_animals():
    """Получить список животных
    ---
    tags:
      - Animals
    responses:
      200:
        description: Список животных
        schema:
          type: array
          items:
            type: object
            properties:
              id:
                type: integer
              name:
                type: string
              type:
                type: string
              age:
                type: integer
              arrival_date:
                type: string
              health:
                type: string
    """
    start_time = time.time()
    try:
        conn = get_db()
        cur = conn.cursor()
        cur.execute("SELECT * FROM animals;")
        animals = cur.fetchall()
        return jsonify([{
            'id': a[0],
            'name': a[1],
            'type': a[2],
            'age': a[3],
            'arrival_date': a[4].isoformat() if a[4] else None,
            'health': a[5]
        } for a in animals])
    except Exception as e:
        return jsonify({"error": str(e)}), 500
    finally:
        REQUEST_LATENCY.labels('/animals').observe(time.time() - start_time)
        if 'conn' in locals():
            conn.close()

@app.route('/api/animals', methods=['POST'])
def add_animal():
    """Добавить новое животное
    ---
    tags:
      - Animals
    parameters:
      - in: body
        name: animal
        required: true
        schema:
          type: object
          required:
            - name
            - type
            - age
            - health
          properties:
            name:
              type: string
            type:
              type: string
            age:
              type: integer
            health:
              type: string
    responses:
      201:
        description: Животное добавлено
      400:
        description: Неверный JSON
      500:
        description: Ошибка сервера
    """
    app.logger.info(f"Received data: {request.json}")
    start_time = time.time()
    if not request.json:
        return jsonify({"error": "JSON data required"}), 400

    try:
        data = request.json
        conn = get_db()
        cur = conn.cursor()
        cur.execute(
            """INSERT INTO animals 
            (name, animal_type, age, health_status) 
            VALUES (%s, %s, %s, %s) RETURNING id""",
            (data.get('name'), data.get('type'), data.get('age'), data.get('health'))
        )
        conn.commit()
        return jsonify({"id": cur.fetchone()[0]}), 201
    except Exception as e:
        return jsonify({"error": str(e)}), 500
    finally:
        REQUEST_LATENCY.labels('/api/animals').observe(time.time() - start_time)
        if 'conn' in locals():
            conn.close()

@app.route('/slow')
def slow_endpoint():
    """Медленный эндпоинт для теста алертов
    ---
    tags:
      - Debug
    responses:
      200:
        description: Медленный ответ
    """
    start_time = time.time()
    delay = random.uniform(0.7, 0.9)
    time.sleep(delay)
    REQUEST_LATENCY.labels('/slow').observe(time.time() - start_time)
    return jsonify({"status": "slow", "delay": delay})
@app.route('/main')
def main_ui():
    try:
        conn = get_db()
        cur = conn.cursor()
        cur.execute("SELECT * FROM animals;")
        animals = cur.fetchall()
    except Exception as e:
        return f"<h1>Ошибка подключения к БД: {e}</h1>"
    finally:
        if 'conn' in locals():
            conn.close()

    return render_template_string("""
    <!DOCTYPE html>
    <html>
    <head>
        <title>Животные</title>
        <style>
            body { font-family: sans-serif; padding: 2em; background: #f9f9f9; }
            h1 { color: #4b0082; }
            table { width: 100%; border-collapse: collapse; background: white; }
            th, td { padding: 10px; border: 1px solid #ccc; }
            th { background-color: #f0f0f0; }
            tr:nth-child(even) { background-color: #f9f9f9; }
        </style>
    </head>
    <body>
        <h1>🐾 Список животных</h1>
        <table>
            <thead>
                <tr>
                    <th>ID</th><th>Имя</th><th>Тип</th><th>Возраст</th><th>Дата</th><th>Здоровье</th>
                </tr>
            </thead>
            <tbody>
            {% for a in animals %}
                <tr>
                    <td>{{ a[0] }}</td>
                    <td>{{ a[1] }}</td>
                    <td>{{ a[2] }}</td>
                    <td>{{ a[3] }}</td>
                    <td>{{ a[4].isoformat() if a[4] else '' }}</td>
                    <td>{{ a[5] }}</td>
                </tr>
            {% endfor %}
            </tbody>
        </table>
    </body>
    </html>
    """, animals=animals)

@app.route('/metrics')
def metrics():
    """Метрики Prometheus"""
    return Response(generate_latest(), mimetype=CONTENT_TYPE_LATEST)


@app.after_request
def track_metrics(response):
    REQUEST_COUNT.labels(request.method, request.path).inc()
    return response

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=8000)
