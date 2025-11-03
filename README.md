# Sistema Distribuido - Integrales Definidas

## Configuración de Red

**IPs asignadas:**
- 172.20.1.158/24 - Master (RPC: 3410, API: 8080)
- 172.20.1.160/24 - Worker 1
- 172.20.1.161/24 - Worker 2

## Inicio

### 1. Iniciar el Master (VM 172.20.1.158)
```powershell
cd master
go run main.go
```
El master escuchará en:
- RPC: `172.20.1.158:3410` (para workers)
- API: `172.20.1.158:8080` (para control HTTP)

### 2. Iniciar los Workers

#### Worker 1 (VM 172.20.1.160):
```powershell
cd worker
go run main.go
```

#### Worker 2 (VM 172.20.1.161):
```powershell
cd worker
go run main.go
```

Los workers se conectarán automáticamente a `172.20.1.158:3410` y se denominarán: `worker-1`, `worker-2`, etc.

## Uso de la API

Todos los comandos deben apuntar a la IP del master: **172.20.1.158:8080**

### Opción 1: División Automática

Un solo POST y el sistema divide automáticamente entre los workers:

**POST** : http://172.20.1.158:8080/integrals

```bash
    {
    "function": "x^2",
    "lower_bound": 0.0,
    "upper_bound": 4.0,
    "interval_size": 0.01,
    "auto_divide": true
    }
```

**Respuesta:**
```json
{
  "message": "Integral auto-divided among workers",
  "function": "x^2",
  "range": [0.0, 4.0],
  "interval_size": 0.01,
  "workers_assigned": 2
}
```

### Opción 2: Asignación Manual Múltiple

Asigna múltiples rangos en un solo POST:
(Si se quisiera asignar distintos intervalos a los workers)

**POST:** http://172.20.1.158:8080/ranges/assign-multi 

```bash
    {
    "function": "x^2",
    "interval_size": 0.01,
    "ranges": [
      {
        "worker_name": "worker-1",
        "lower_bound": 0.0,
        "upper_bound": 2.0
      },
      {
        "worker_name": "worker-2",
        "lower_bound": 2.0,
        "upper_bound": 4.0
      }
    ]
  }
```

### Opción 3: Asignación Individual

Asigna rangos uno por uno:

**POST:** http://172.20.1.158:8080/ranges/assign

```bash
# Worker 1
  {
    "worker_name": "worker-1",
    "function": "x^2",
    "lower_bound": 0.0,
    "upper_bound": 2.0,
    "interval_size": 0.01
  }

# Worker 2
  {
    "worker_name": "worker-2",
    "function": "x^2",
    "lower_bound": 2.0,
    "upper_bound": 4.0,
    "interval_size": 0.01
  }
```

## Endpoints Disponibles

| Método | Endpoint | Descripción |
|--------|----------|-------------|
| POST | `/integrals` | Inicia integral (con `auto_divide=true` divide automáticamente) |
| POST | `/ranges/assign` | Asigna rango a un worker |
| POST | `/ranges/assign-multi` | Asigna múltiples rangos en un POST |
| GET | `/ranges/list` | Lista todos los rangos asignados y su progreso |
| POST | `/ranges/clear` | Limpia todas las asignaciones |
| GET | `/workers` | Lista workers conectados |
| GET | `/result` | Obtiene el resultado de la integral |
| GET | `/stats` | Estadísticas de distribución de jobs |
| GET | `/health` | Health check |

## Monitoreo

### Ver Workers Conectados
```bash
curl http://172.20.1.158:8080/workers
```

### Ver Rangos Asignados
```bash
curl http://172.20.1.158:8080/ranges/list
```

### Ver Resultado
```bash
curl http://172.20.1.158:8080/result?precision=10
```

### Ver Estadísticas
```bash
curl http://172.20.1.158:8080/stats
```
