# Guía de Integración Backend: All Fitness LLM Service

Este documento detalla la especificación técnica completa y el protocolo de comunicación para que el **Backend Principal** de All Fitness se integre de manera correcta y eficiente con el **LLM Service (AI Gateway)**.

---

## 1. Arquitectura y Protocolo de Comunicación

El servicio funciona mediante un **patrón de mensajería asíncrona basado en colas de Redis**. No expone endpoints HTTP sincrónicos, lo que previene problemas de timeouts derivados de la latencia natural de los modelos de lenguaje (LLMs) y garantiza una alta resiliencia.

### Diagrama de Flujo

```
┌─────────────────┐                               ┌─────────────────┐
│                 │─── 1. LPUSH (Task JSON) ────>│  Cola de Redis  │
│  Backend Core   │                               │  (Tasks Queue)  │
│                 │                               └────────┬────────┘
│                 │                                        │
│                 │                                  2. BRPOP (Worker)
│                 │                                        │
│                 │                                        ▼
│                 │                               ┌─────────────────┐
│                 │                               │   LLM Service   │
│                 │                               │   (AI Worker)   │
│                 │                               └────────┬────────┘
│                 │                                        │
│                 │                                  3. SET result
│                 │                                        ▼
│                 │                               ┌─────────────────┐
│                 │<─── 4. GET (Poll/Fetch) ──────│  Almacén Redis  │
│                 │    allfitness:results:{id}    │    (Results)    │
└─────────────────┘                               └─────────────────┘
```

### Configuración de Conexión a Redis

| Parámetro | Valor por Defecto | Descripción |
| :--- | :--- | :--- |
| **Cola de Entrada (Tareas)** | `allfitness:tasks:queue` | Lista de Redis donde el backend publica las peticiones (`LPUSH`). |
| **Prefijo de Resultados** | `allfitness:results:` | Prefijo de clave donde el worker guarda la respuesta (`SET`). |
| **Clave Final de Resultado** | `allfitness:results:{request_id}` | Clave exacta para consultar el resultado de una petición específica. |
| **TTL del Resultado** | `15 minutos` (`900s`) | Tiempo durante el cual el resultado permanece en Redis antes de expirar. |

---

## 2. Formato de Petición (`TaskPayload`)

Para solicitar una generación, el backend debe serializar en formato JSON un objeto `TaskPayload` y hacer `LPUSH` a la cola `allfitness:tasks:queue`.

### Estructura General

```json
{
  "request_id": "string (UUID v4 único por petición)",
  "request_type": "GENERATE_WORKOUT | GENERATE_RECIPE | GENERATE_EXERCISE",
  "created_at": "string (ISO 8601 / RFC 3339, ej: 2026-11-02T10:00:00Z)",
  "user_profile": {
    "user_id": "string (ID del usuario en el backend)",
    "age": 28,
    "height": 178.5,
    "weight": 76.0,
    "experience_level": "beginner | intermediate | advanced",
    "goal": "string (meta principal del usuario)",
    "time_availability": 45,
    "diet_description": "string (opcional: preferencias o restricciones dietéticas)",
    "objective_description": "string (opcional: enfoque particular o detalles de la solicitud)"
  },
  "available_equipment": [
    "Dumbbells",
    "Bench",
    "Pull-up Bar"
  ],
  "parameters": {
    "clave_adicional": "valor_adicional"
  }
}
```

### Campos Detallados

| Campo | Tipo | Requerido | Descripción |
| :--- | :--- | :---: | :--- |
| `request_id` | `string` | **Sí** | Identificador único generado por el backend (se recomienda UUID v4). Se usa para correlacionar la respuesta. |
| `request_type` | `string` | **Sí** | Tipo de tarea. Valores permitidos: `GENERATE_WORKOUT`, `GENERATE_RECIPE`, `GENERATE_EXERCISE`. |
| `created_at` | `string` | No | Timestamp de emisión en formato ISO 8601 / RFC 3339. Si se omite, el worker asigna el tiempo actual UTC. |
| `user_profile` | `object` | **Sí** | Atributos biométricos y objetivos del usuario. |
| `user_profile.user_id` | `string` | No | Identificador del usuario para auditoría y trazabilidad. |
| `user_profile.age` | `int` | No | Edad del usuario en años. |
| `user_profile.height` | `float` | No | Estatura en centímetros (ej: `180.0`). |
| `user_profile.weight` | `float` | No | Peso corporal en kilogramos (ej: `75.5`). |
| `user_profile.experience_level` | `string` | No | Nivel de entrenamiento: `beginner`, `intermediate` o `advanced`. |
| `user_profile.goal` | `string` | No | Meta principal (ej: `Hipertrofia`, `Pérdida de grasa`, `Fuerza general`). |
| `user_profile.time_availability` | `int` | No | Tiempo disponible en minutos para la sesión (relevante para workouts). |
| `user_profile.diet_description` | `string` | No | Preferencias alimenticias, alergias o intolerancias (relevante para recetas). |
| `user_profile.objective_description` | `string` | No | Detalle en lenguaje natural de lo que el usuario desea lograr. |
| `available_equipment` | `array[string]` | No | Lista de equipamiento disponible. Si está vacío o nulo, el sistema asume peso corporal (`bodyweight`). |
| `parameters` | `object` | No | Diccionario clave-valor para parámetros arbitrarios complementarios. |

---

## 3. Tipos de Tareas y Ejemplos de Payload

### 3.1. `GENERATE_WORKOUT` (Rutina de Entrenamiento)
Genera una sesión de ejercicios adaptada a nivel, tiempo disponible y material.

```json
{
  "request_id": "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
  "request_type": "GENERATE_WORKOUT",
  "created_at": "2026-11-02T15:30:00Z",
  "user_profile": {
    "user_id": "usr_99812",
    "age": 27,
    "height": 180,
    "weight": 78.5,
    "experience_level": "intermediate",
    "goal": "Hipertrofia de torso",
    "time_availability": 50,
    "objective_description": "Enfoque en pecho superior y deltoides lateral"
  },
  "available_equipment": [
    "Dumbbells",
    "Incline Bench",
    "Resistance Bands"
  ]
}
```

---

### 3.2. `GENERATE_RECIPE` (Receta Nutricional)
Genera una receta balanceada con cálculo de macronutrientes acorde a las metas del usuario.

```json
{
  "request_id": "a82c6104-5e12-4011-8be5-9f5647a2119b",
  "request_type": "GENERATE_RECIPE",
  "created_at": "2026-11-02T15:30:00Z",
  "user_profile": {
    "user_id": "usr_99812",
    "age": 27,
    "height": 180,
    "weight": 78.5,
    "goal": "Definición muscular",
    "diet_description": "Alto en proteína, sin lácteos, prefiere salmón o pechuga de pollo",
    "objective_description": "Almuerzo rápido de menos de 600 calorías con mínimo 45g de proteína"
  }
}
```

---

### 3.3. `GENERATE_EXERCISE` (Guía de Ejercicio Individual)
Genera las instrucciones biomecánicas, postura, ejecución técnica y consejos de seguridad para un movimiento específico.

```json
{
  "request_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
  "request_type": "GENERATE_EXERCISE",
  "created_at": "2026-11-02T15:30:00Z",
  "user_profile": {
    "user_id": "usr_99812",
    "experience_level": "beginner",
    "goal": "Aprender técnica básica de sentadilla",
    "objective_description": "Molestia leve en tobillos al descender, necesita variantes seguras"
  },
  "available_equipment": [
    "Kettlebell"
  ]
}
```

---

## 4. Formato de Respuesta (`TaskResult`)

Cuando el worker finaliza el procesamiento, guarda el resultado en Redis bajo la clave:
```
allfitness:results:{request_id}
```

El backend debe leer esta clave mediante un comando `GET`.

### Estructura del Envelope de Respuesta

```json
{
  "request_id": "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
  "request_type": "GENERATE_WORKOUT",
  "status": "SUCCESS",
  "data": { ... },
  "error_code": "",
  "error_message": "",
  "execution_time_ms": 3420,
  "model_used": "anthropic/claude-3.5-sonnet",
  "repair_attempts": 0,
  "completed_at": "2026-11-02T15:30:04.120Z"
}
```

### Campos del Envelope

| Campo | Tipo | Descripción |
| :--- | :--- | :--- |
| `request_id` | `string` | Mismo ID enviado en la petición original. |
| `request_type` | `string` | Tipo de tarea procesada (`GENERATE_WORKOUT`, etc.). |
| `status` | `string` | Estado final: `"SUCCESS"` o `"FAILED"`. |
| `data` | `object` | Carga útil con la respuesta estructurada validada. **Presente sólo si `status == "SUCCESS"`**. |
| `error_code` | `string` | Código de error estandarizado. Vacío en caso de éxito. |
| `error_message` | `string` | Descripción legible del error. Vacío en caso de éxito. |
| `execution_time_ms`| `int64` | Tiempo total transcurrido en el microservicio en milisegundos. |
| `model_used` | `string` | Identificador del modelo que resolvió la petición (ej: modelo principal o modelo de fallback). |
| `repair_attempts` | `int` | Cantidad de reintentos automáticos de reparación de esquema ejecutados por el servicio (0 a 2). |
| `completed_at` | `string` | Fecha y hora en formato ISO 8601 en que se persistió el resultado. |

---

## 5. Esquemas de Datos (`data`) según la Tarea

Cuando `status == "SUCCESS"`, el objeto dentro de `data` tiene un esquema estrictamente tipado y validado:

### 5.1. Datos de `GENERATE_WORKOUT`
```json
{
  "name": "Rutina de Torso Enfoque Superior",
  "estimated_duration": 45,
  "exercises": [
    {
      "exercise_name": "Press Inclinado con Mancuernas",
      "sets": 4,
      "reps": "8-10",
      "rest_seconds": 90,
      "notes": "Mantener retracción escapular y banco a 30 grados."
    },
    {
      "exercise_name": "Elevaciones Laterales con Mancuerna",
      "sets": 4,
      "reps": "12-15",
      "rest_seconds": 60,
      "notes": "Controlar la fase excéntrica en 2 segundos."
    }
  ]
}
```

### 5.2. Datos de `GENERATE_RECIPE`
```json
{
  "title": "Salmón Grillado con Quinoa y Espárragos",
  "description": "Plato alto en ácidos grasos omega-3 y proteína de alto valor biológico.",
  "ingredients": "• 200g Filete de salmón fresco\n• 80g Quinoa en seco\n• 100g Espárragos trigueros\n• 1 cda Aceite de oliva virgen extra\n• Sal, pimienta y limón al gusto",
  "preparation_time": 25,
  "calories": 540,
  "protein": 46.5,
  "carbs": 38.0,
  "fats": 22.0,
  "instructions": [
    "Enjuagar y cocinar la quinoa en agua hirviendo durante 12 minutos.",
    "Sellar el filete de salmón en sartén antiadherente 4 minutos por lado.",
    "Saltear los espárragos con aceite de oliva a fuego medio.",
    "Servir el salmón sobre la cama de quinoa y acompañar con los espárragos y limón."
  ]
}
```

### 5.3. Datos de `GENERATE_EXERCISE`
```json
{
  "name": "Goblet Squat (Sentadilla Copa)",
  "category": "strength",
  "muscle_group": "Cuádriceps, Glúteos, Core",
  "difficulty": "beginner",
  "equipment_needed": "Kettlebell o Mancuerna",
  "instructions": [
    "Sostener la pesa verticalmente contra el pecho con ambas manos pegadas al cuerpo.",
    "Separar los pies al ancho de hombros con puntas ligeramente rotadas hacia afuera.",
    "Iniciar el descenso empujando la cadera hacia atrás y flexionando rodillas.",
    "Descender hasta que los codos toquen la cara interna de los muslos manteniendo torso erguido.",
    "Empujar contra el piso con todo el pie para volver a la posición inicial."
  ],
  "tips": [
    "Mantener la mirada al frente para evitar arquear la columna dorsal.",
    "No permitir que las rodillas colapsen hacia adentro (valgo de rodilla)."
  ]
}
```

---

## 6. Manejo de Errores y Seguridad

Si una petición falla, `status` será `"FAILED"` y el campo `error_code` indicará la causa:

| `error_code` | Causa | Acción recomendada en el Backend |
| :--- | :--- | :--- |
| `PROMPT_INJECTION_DETECTED` | Se detectaron patrones de ataque (inyección de prompt, jailbreak tipo DAN, solicitudes de volcado de instrucciones del sistema, etc.). | Responder al usuario con un error `400 Bad Request` indicando que el texto contiene instrucciones no permitidas. |
| `INVALID_PAYLOAD` | Faltan campos requeridos como `request_id` o el JSON está corrupto. | Revisar la estructura enviada desde el backend antes de hacer `LPUSH`. |
| `UNSUPPORTED_REQUEST_TYPE` | El valor de `request_type` no coincide con los tipos soportados. | Asegurar que `request_type` sea uno de los valores permitidos (`GENERATE_WORKOUT`, `GENERATE_RECIPE`, `GENERATE_EXERCISE`). |
| `LLM_API_ERROR` | Fallo de conexión o disponibilidad de los proveedores de LLM tras agotarse los reintentos y el modelo de fallback. | Reintentar más tarde o retornar error `503 Service Unavailable` al cliente. |
| `SCHEMA_VALIDATION_FAILED` | La respuesta del modelo no cumplió con el contrato JSON requerido incluso tras las rondas de auto-reparación. | Reintentar la petición generando un nuevo `request_id`. |
| `INTERNAL_ERROR` | Error imprevisto interno del worker al compilar plantillas o serializar datos. | Notificar al equipo de infraestructura para revisar logs de `cmd/worker`. |

### Sanitización Automática de PII (Privacidad)
Antes de enviar el prompt a los modelos de lenguaje, el servicio **ofusca automáticamente información sensible (PII)**:
* Direcciones de correo electrónico se reemplazan por `[EMAIL REDACTED]`.
* Números de teléfono se reemplazan por `[PHONE REDACTED]`.
* Números de tarjetas o identificaciones sensibles se ofuscan.

---

## 7. Estrategia Recomendada de Consumo en el Backend

Dado que los LLMs tardan habitualmente entre **1.5 y 6 segundos** en responder:

1. **Generar `request_id` único**: Usar UUID v4.
2. **Publicar tarea**: Ejecutar `LPUSH allfitness:tasks:queue <json>`.
3. **Polling con Timeout**:
   - Consultar `GET allfitness:results:{request_id}` cada `1` segundo.
   - Establecer un timeout máximo de espera en el backend (recomendado: **30 a 45 segundos**).
4. **Respuesta inmediata al frontend**:
   - **Opción A (Sincrónica para el cliente)**: El backend mantiene la petición HTTP abierta mientras hace el polling de Redis durante unos segundos y responde directamente cuando el resultado esté disponible.
   - **Opción B (Asincrónica vía Webhook / Polling de frontend)**: El backend responde de inmediato con `202 Accepted` y el `request_id`, permitiendo al frontend consultar el estado de la tarea.

---

## 8. Ejemplos de Implementación en Backend

### 8.1. Ejemplo en Node.js / TypeScript (`ioredis`)

```typescript
import Redis from "ioredis";
import { randomUUID } from "crypto";

const redis = new Redis({
  host: process.env.REDIS_HOST || "localhost",
  port: Number(process.env.REDIS_PORT) || 6379,
  password: process.env.REDIS_PASSWORD || undefined,
});

interface GenerateWorkoutInput {
  userId: string;
  age: number;
  height: number;
  weight: number;
  experienceLevel: "beginner" | "intermediate" | "advanced";
  goal: string;
  timeAvailability: number;
  availableEquipment: string[];
}

export async function requestWorkoutGeneration(input: GenerateWorkoutInput) {
  const requestId = randomUUID();
  const queueKey = "allfitness:tasks:queue";
  const resultKey = `allfitness:results:${requestId}`;

  const taskPayload = {
    request_id: requestId,
    request_type: "GENERATE_WORKOUT",
    created_at: new Date().toISOString(),
    user_profile: {
      user_id: input.userId,
      age: input.age,
      height: input.height,
      weight: input.weight,
      experience_level: input.experienceLevel,
      goal: input.goal,
      time_availability: input.timeAvailability,
    },
    available_equipment: input.availableEquipment,
  };

  // 1. Enviar tarea a la cola
  await redis.lpush(queueKey, JSON.stringify(taskPayload));

  // 2. Polling con timeout de 40 segundos
  const maxWaitMs = 40_000;
  const pollIntervalMs = 1_000;
  const startTime = Date.now();

  while (Date.now() - startTime < maxWaitMs) {
    const rawResult = await redis.get(resultKey);
    if (rawResult) {
      const result = JSON.parse(rawResult);
      if (result.status === "SUCCESS") {
        return result.data; // WorkoutResponse
      }
      throw new Error(`[${result.error_code}] ${result.error_message}`);
    }
    await new Promise((res) => setTimeout(res, pollIntervalMs));
  }

  throw new Error("Timeout: La generación de la rutina tomó demasiado tiempo");
}
```

---

### 8.2. Ejemplo en Python (`redis-py`)

```python
import json
import time
import uuid
from typing import Any, Dict
import redis

r = redis.Redis(
    host="localhost",
    port=6379,
    password=None,
    decode_responses=True
)

def request_recipe_generation(user_id: str, goal: str, diet_description: str) -> Dict[str, Any]:
    request_id = str(uuid.uuid4())
    queue_name = "allfitness:tasks:queue"
    result_key = f"allfitness:results:{request_id}"

    payload = {
        "request_id": request_id,
        "request_type": "GENERATE_RECIPE",
        "created_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "user_profile": {
            "user_id": user_id,
            "goal": goal,
            "diet_description": diet_description
        }
    }

    # 1. Publicar tarea
    r.lpush(queue_name, json.dumps(payload))

    # 2. Esperar resultado (polling con timeout de 40 segundos)
    timeout = 40
    start = time.time()

    while time.time() - start < timeout:
        raw_res = r.get(result_key)
        if raw_res:
            res = json.loads(raw_res)
            if res["status"] == "SUCCESS":
                return res["data"]
            raise Exception(f"[{res.get('error_code')}] {res.get('error_message')}")
        time.sleep(1)

    raise TimeoutError("Tiempo de espera agotado esperando el resultado del LLM")
```

---

### 8.3. Ejemplo en Go (`go-redis/v9`)

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type WorkoutResult struct {
	Name              string `json:"name"`
	EstimatedDuration int    `json:"estimated_duration"`
	Exercises         []struct {
		ExerciseName string `json:"exercise_name"`
		Sets         int    `json:"sets"`
		Reps         string `json:"reps"`
		RestSeconds  int    `json:"rest_seconds"`
		Notes        string `json:"notes"`
	} `json:"exercises"`
}

func RequestWorkout(ctx context.Context, rdb *redis.Client) (*WorkoutResult, error) {
	reqID := uuid.New().String()
	queueKey := "allfitness:tasks:queue"
	resultKey := fmt.Sprintf("allfitness:results:%s", reqID)

	task := map[string]interface{}{
		"request_id":   reqID,
		"request_type": "GENERATE_WORKOUT",
		"created_at":   time.Now().UTC().Format(time.RFC3339),
		"user_profile": map[string]interface{}{
			"user_id":           "usr-123",
			"age":               25,
			"height":            175.0,
			"weight":            70.0,
			"experience_level":  "intermediate",
			"goal":              "Fuerza e Hipertrofia",
			"time_availability": 45,
		},
		"available_equipment": []string{"Dumbbells", "Pull-up Bar"},
	}

	data, err := json.Marshal(task)
	if err != nil {
		return nil, err
	}

	if err := rdb.LPush(ctx, queueKey, data).Err(); err != nil {
		return nil, fmt.Errorf("error publicando en cola: %w", err)
	}

	// Esperar resultado
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	timeout := time.After(40 * time.Second)

	for {
		select {
		case <-timeout:
			return nil, fmt.Errorf("timeout esperando respuesta")
		case <-ticker.C:
			raw, err := rdb.Get(ctx, resultKey).Result()
			if err == nil {
				var envelope struct {
					Status       string          `json:"status"`
					ErrorCode    string          `json:"error_code"`
					ErrorMessage string          `json:"error_message"`
					Data         json.RawMessage `json:"data"`
				}
				if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
					return nil, err
				}
				if envelope.Status == "SUCCESS" {
					var workout WorkoutResult
					if err := json.Unmarshal(envelope.Data, &workout); err != nil {
						return nil, err
					}
					return &workout, nil
				}
				return nil, fmt.Errorf("[%s] %s", envelope.ErrorCode, envelope.ErrorMessage)
			}
		}
	}
}
```

