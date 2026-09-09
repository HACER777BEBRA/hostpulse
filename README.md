# HostPulse

Telegram-бот для мониторинга Linux-сервера.

Собирает системные метрики, проверяет состояние systemd-сервисов и Docker-контейнеров, отправляет периодические отчёты и алерты при превышении порогов, а также уведомляет о перезагрузке хоста.

## Возможности

- **Метрики** — CPU, RAM, диск, load average, топ процессов
- **Сеть** — скорость ↓↑ сейчас и за окна 1 мин / 15 мин / 1 час
- **systemd** — статусы юнитов из вашего списка
- **Docker** — проверка нужных контейнеров и полный `docker ps -a`
- **Автоотчёты** — по расписанию, краткие или развёрнутые
- **Алерты** — пороги CPU / RAM / диск / load с кулдауном и mute
- **Ребут** — уведомление при смене boot id
- **Восстановление** — команда `/restart`: опциональный скрипт + `docker restart`

Списки сервисов и контейнеров задаются в `config.yaml` под конкретный хост.

## Команды

| Команда | Действие |
|---|---|
| `/status` | сводка: CPU, RAM, диск, сеть, сервисы |
| `/report` | полный снимок сейчас |
| `/network` | нагрузка сети по окнам |
| `/processes` | топ процессов |
| `/docker` | `docker ps -a` |
| `/services` | статусы systemd |
| `/reboot_info` | boot time, uptime, boot id |
| `/interval 24h` | частота автоотчётов |
| `/remind short` / `full` | формат автонапоминания |
| `/restart` | скрипт + перезапуск контейнеров из списка |
| `/mute 1h` / `/unmute` | алерты |
| `/help` / `/help full` | справка |

## Конфиг

Два файла:

| Файл | Что внутри |
|---|---|
| `.env` | `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID` |
| `config.yaml` | контейнеры, сервисы, пороги, интервалы |

Приоритет: env / `.env` → `config.yaml` → значения по умолчанию.

```yaml
watch:
  services: [docker, ssh, nginx]
  containers: [nginx, postgres, redis]

alerts:
  cpu: 85
  ram: 90
  disk: 90
  load: 2.0
```

Полный шаблон: [`config.example.yaml`](config.example.yaml).

## Развёртывание

Нужен Linux с systemd; Docker — по желанию. Go на сервере не обязателен.

### Из релиза

1. Скачайте бинарник и архив конфигов с [Releases](https://github.com/HACER777BEBRA/hostpulse/releases) (`linux-amd64` или `linux-arm64`).
2. Распакуйте, скопируйте примеры: `.env.example` → `.env`, `config.example.yaml` → `config.yaml`.
3. Заполните токен/chat id и списки в `config.yaml`.
4. Установите:

```bash
sudo mkdir -p /opt/hostpulse /var/lib/hostpulse
sudo cp hostpulse .env config.yaml /opt/hostpulse/
sudo chmod 600 /opt/hostpulse/.env
sudo cp hostpulse.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now hostpulse
```

Дальше правки — в `/opt/hostpulse/config.yaml`, затем `sudo systemctl restart hostpulse`.

### Из исходников

```bash
git clone https://github.com/HACER777BEBRA/hostpulse.git
cd hostpulse && bash scripts/setup.sh
make test && make build    # или make build-linux / make install
./hostpulse .env
```

Для локального теста на Windows: `STATE_FILE=./state.json`.
