# HostPulse

Telegram-бот мониторинга Linux-сервера: CPU, RAM, диск, load, сеть, systemd и Docker. Периодические отчёты, алерты по порогам и уведомление о ребуте.

## Быстрый старт на новом ПК / сервере

```bash
git clone https://github.com/HACER777BEBRA/hostpulse.git
cd hostpulse
bash scripts/setup.sh
```

Скрипт создаст локальные файлы из примеров:

| Файл | Назначение |
|---|---|
| `.env` | секреты Telegram |
| `config.yaml` | списки контейнеров/сервисов, пороги, скрипт restart |

Отредактируйте под хост:

```bash
nano .env              # TELEGRAM_BOT_TOKEN, TELEGRAM_CHAT_ID
nano config.yaml       # watch.containers, watch.services
```

Пример списка контейнеров в `config.yaml`:

```yaml
watch:
  services:
    - docker
    - ssh
    - nginx
  containers:
    - nginx
    - postgres
    - redis
```

Сборка и установка:

```bash
make test
make build            # локальный бинарник ./hostpulse
make build-linux      # кросс-сборка с Windows/macOS → hostpulse-linux
make install          # Linux: /opt/hostpulse + systemd
```

Запуск без systemd:

```bash
./hostpulse .env
```

На Windows для локального теста в `config.yaml` или `.env` поставьте `paths.state_file: ./state.json` / `STATE_FILE=./state.json`.

## Команды Telegram

| Команда | Что делает |
|---|---|
| `/status` | CPU, RAM, диск, сеть, сервисы |
| `/network` | нагрузка ↓↑ сейчас / 1 мин / 15 мин / 1 час |
| `/processes` | топ процессов по CPU и RAM |
| `/docker` | `docker ps -a` |
| `/services` | статусы systemd из `watch.services` |
| `/reboot_info` | boot time, uptime, boot id |
| `/report` | полный отчёт |
| `/interval 24h` | частота автоотчётов |
| `/restart` | опциональный скрипт + `docker restart` контейнеров из списка |
| `/mute 1h` | выключить алерты |
| `/unmute` | включить алерты |
| `/help` | средняя справка |
| `/help full` | развёрнутая справка |
| `/remind` / `/remind short` / `/remind full` | формат автонапоминания |

Имя команды `/restart` меняется через `restart.command` в YAML (алиасы `/restart` и `/restart_amnezia` тоже принимаются).

## Конфигурация

Приоритет: **переменные окружения / `.env` > `config.yaml` > значения по умолчанию**.

Секреты лучше держать в `.env`, списки и пороги — в `config.yaml` (его удобно копировать на другой хост и править).

| Ключ YAML / env | Смысл |
|---|---|
| `watch.services` / `WATCH_SERVICES` | systemd-юниты (список или CSV) |
| `watch.containers` / `WATCH_CONTAINERS` | docker-контейнеры |
| `restart.script` / `RESTART_SCRIPT` | скрипт перед `docker restart` (пусто = пропуск) |
| `restart.command` / `RESTART_COMMAND` | имя команды в Telegram |
| `alerts.*` / `CPU_ALERT` … | пороги алертов |
| `report_interval` / `REPORT_INTERVAL` | период автоотчёта |
| `paths.state_file` / `STATE_FILE` | файл состояния |
| `CONFIG_FILE` | путь к YAML, если не рядом с `.env` |

Полный шаблон: [`config.example.yaml`](config.example.yaml).

## Структура

```
cmd/monitor       точка входа
internal/config   .env + config.yaml
internal/telegram Bot API
internal/metrics  CPU/RAM/диск/процессы
internal/netload  скорость сети
internal/health   systemd + docker
internal/reboot   boot id
internal/state    mute / cooldown / interval
internal/monitor  команды и циклы
deploy/           systemd unit
scripts/          setup на новом хосте
```

## Требования

- Go 1.22+
- Linux-сервер с systemd (для мониторинга сервисов) и при желании Docker
