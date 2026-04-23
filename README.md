# OPC-UA Proxy

Прокси для чтения данных с OPC-UA серверов и отправки по UDP.

## Быстрый старт

### 1. Сервер (принимает UDP, экспортирует OPC UA)

```bash
go run cmd/server/main.go
```

### 2. Клиент (читает с OPC-UA и отправляет на UDP сервер)

```bash
go run cmd/client/main.go -discover-nodes=true
```

## Команды запуска

### Сервер

| Параметр | Описание | По умолчанию |
|----------|----------|-------------|
| `-addr` | UDP адрес для приёма | `:50001` |
| `-opc-addr` | OPC UA адрес | `0.0.0.0` |
| `-opc-port` | OPC UA порт | `4840` |

```bash
# Сервер на порту 4841
go run cmd/server/main.go -opc-port=4841

# Сервер на UDP порту 8001
go run cmd/server/main.go -addr=:8001 -opc-port=4842
```

### Клиент

| Параметр | Описание | По умолчанию |
|----------|----------|-------------|
| `-endpoint` | OPC UA сервер | `opc.tcp://localhost:50000` |
| `-udp` | UDP получатель | `localhost:50001` |
| `-discover-nodes` | Авто-дискавери | `false` |
| `-browse-path` | Путь для браузинга | `ns=0;i=85` |
| `-node-namespace` | Фильтр namespace | `0` (все) |
| `-readonly` | Только читать | `false` |
| `-poll-interval` | Интервал опроса | `50ms` |

```bash
# Читать с Docker OPC-PLC и отправлять на UDP
go run cmd/client/main.go -discover-nodes=true

# Читать с удалённого сервера
go run cmd/client/main.go -endpoint=opc.tcp://192.168.1.100:50000 -discover-nodes=true

# Read-only режим (только читать и логировать)
go run cmd/client/main.go -discover-nodes=true -readonly

# Читать с локального прокси сервера
go run cmd/client/main.go -endpoint=opc.tcp://localhost:4840 -discover-nodes=true -readonly
```

## Примеры использования

### Локальная разработка

```bash
# Терминал 1: OPC-PLC в Docker
docker run -p 50000:50000 mcr.microsoft.com/iotedge/opc-plc

# Терминал 2: Прокси сервер
go run cmd/server/main.go

# Терминал 3: Клиент
go run cmd/client/main.go -discover-nodes=true
```

### Каскад (прокси → прокси)

```bash
# Терминал 1: Docker OPC-PLC
docker run -p 50000:50000 mcr.microsoft.com/iotedge/opc-plc

# Терминал 2: Сервер #1 (принимает :8000, экспортирует :4840)
go run cmd/server/main.go -addr=:8000 -opc-port=4840

# Терминал 3: Клиент #1 (Docker → Сервер #1)
go run cmd/client/main.go -endpoint=opc.tcp://localhost:50000 -discover-nodes=true -udp=:8000

# Терминал 4: Сервер #2 (принимает :9000, экспортирует :4841)
go run cmd/server/main.go -addr=:9000 -opc-port=4841

# Терминал 5: Клиент #2 (Сервер #1 → Сервер #2)
go run cmd/client/main.go -endpoint=opc.tcp://localhost:4840 -discover-nodes=true -udp=:9000
```

### Тестирование

```bash
# Проверить что сервер работает (read-only клиент)
go run cmd/client/main.go -endpoint=opc.tcp://localhost:4840 -discover-nodes=true -readonly
```

## Архитектура

```
[Source OPC-UA Server] :50000
       ↓
[Client] → UDP :8000
       ↓
[Server] → OPC-UA :4840
       ↓
[Клиент для проверки]
```

## Environment Variables

```bash
OPC_ENDPOINT=opc.tcp://server:50000 \
UDP_DEST=localhost:8000 \
POLL_INTERVAL=100ms \
go run cmd/client/main.go -discover-nodes=true
```

## Troubleshooting

### "No suitable endpoint found"

```bash
# Попробуй с явным указанием security
go run cmd/client/main.go -endpoint=opc.tcp://localhost:4840 -discover-nodes=true -sec-mode=None -sec-policy=None
```

### Сервер не виден

```bash
# Проверь что сервер запущен
netstat -tlnp | grep 4840
```