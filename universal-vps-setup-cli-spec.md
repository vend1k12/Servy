# Universal VPS Setup CLI — ТЗ для AI-агентов

## 1. Назначение проекта

Нужно создать open-source CLI-инструмент для первичной настройки VPS/server environment на **Ubuntu и Debian**.

Инструмент запускается **внутри сервера**, а не удалённо по SSH. Его задача — безопасно, повторяемо и понятно подготовить сервер под базовые сценарии разработки и production-хостинга.

Проект должен быть удобен в первую очередь для личного workflow автора, но архитектура и UX должны позволять опубликовать его как open-source инструмент.

---

## 2. Главная идея

Инструмент должен быть не “огромным bash-скриптом”, а полноценным CLI с профилями, проверками, idempotency-логикой и интерактивным wizard.

Рекомендуемая модель:

```text
bootstrap script -> скачивает CLI -> CLI делает всю настройку
```

Bootstrap-скрипт должен быть минимальным. Он не должен выполнять основную настройку сервера. Его задача — определить ОС/архитектуру, скачать актуальный бинарник CLI, положить его в системный PATH и запустить первичную проверку.

Основная логика установки Docker, Caddy, firewall, hardening, Node tooling и прочего должна жить внутри CLI.

---

## 3. Рабочее название

Временное название:

```text
serverctl
```

Можно заменить позже. В коде не нужно жёстко завязываться на финальное название.

---

## 4. Целевые ОС

Версия v1 должна поддерживать:

```text
Ubuntu LTS
Debian stable
```

Минимально целевые системы:

```text
Ubuntu 22.04
Ubuntu 24.04
Debian 12
Debian 13, если актуально и официальные инструкции зависимостей это поддерживают
```

AI-агент перед реализацией обязан проверить актуальные версии и поддерживаемые инструкции установки по официальным источникам.

Docker должен устанавливаться через официальный apt repository по официальным Docker docs. Агент обязан проверить актуальные страницы для Ubuntu и Debian перед реализацией.

Caddy должен устанавливаться только через официально поддерживаемый способ для Debian/Ubuntu/Raspbian packages или альтернативный официальный способ, если выбранная стратегия этого требует.

---

## 5. Стек реализации

Рекомендуемый стек для v1:

```text
Language: Go
CLI framework: Cobra или urfave/cli
Config: YAML
Distribution: GitHub Releases
Install: minimal install.sh
Target: single binary
```

Почему Go:

```text
1. Один бинарник без обязательной runtime-зависимости.
2. Хорошо подходит для системного CLI.
3. Удобно собирать под linux amd64/arm64.
4. Лучше для open-source DevOps-инструмента, чем TypeScript/Bun с runtime-зависимостями.
5. Проще и быстрее для MVP, чем Rust.
```

TypeScript/Bun можно рассматривать как альтернативу, но не как основной вариант для v1. Bun может быть полезен для локальной разработки или отдельных утилит, но CLI настройки сервера не должен требовать Bun как runtime-зависимость.

Rust можно рассматривать для будущей версии, если появится потребность в более строгой типобезопасности, сложном TUI или максимально надёжном standalone-инструменте.

---

## 6. Основной UX

CLI должен поддерживать два режима:

```text
1. Interactive wizard
2. Config-driven apply
```

### 6.1 Interactive wizard

Команда:

```text
serverctl init
```

Поведение:

```text
1. Опрашивает пользователя.
2. Определяет ОС, пользователя, права, package manager.
3. Предлагает профиль установки.
4. Предлагает опциональные модули.
5. Показывает итоговый план.
6. Генерирует config-файл.
7. Предлагает применить настройку.
```

Wizard не должен сразу молча менять сервер. Перед применением должен быть понятный summary.

### 6.2 Config-driven режим

Команда:

```text
serverctl apply --config <file>
```

Поведение:

```text
1. Читает YAML config.
2. Валидирует схему.
3. Проверяет совместимость с текущей ОС.
4. Строит execution plan.
5. Показывает план.
6. Выполняет шаги.
7. Логирует результат.
8. Показывает финальный status report.
```

### 6.3 Dry-run

Обязательная команда/флаг:

```text
serverctl apply --dry-run
```

Dry-run должен показать, что будет изменено, но не менять систему.

---

## 7. Профили установки

В v1 нужны только серверные профили. Не нужно делать профили под конкретные типы проектов вроде TMA, Mailpit, лендингов и т.д.

### 7.1 `docker-only`

Назначение: минимальная подготовка сервера под Docker-проекты.

Состав:

```text
Docker Engine
Docker Compose plugin
Базовые пакеты
Опциональный firewall
Опциональный hardening
Опциональный swap
Опциональный deploy user
```

Docker Compose должен ставиться как актуальный Docker Compose plugin по официальной схеме Docker, а не как устаревший отдельный `docker-compose` binary, если официальные Docker docs не рекомендуют иное на момент реализации.

### 7.2 `node`

Назначение: сервер для Node/Bun/pnpm-based окружения, если часть процессов запускается не только в Docker.

Состав:

```text
Всё из docker-only
nvm
Node.js через nvm
pnpm
bun
Опциональный Caddy
Опциональный firewall
Опциональный hardening
```

Важно: профиль `node` не должен быть дефолтным. Если деплой идёт только через Docker, установка Node/Bun/pnpm/nvm на host необязательна.

nvm должен использоваться как Node Version Manager для установки и переключения версий Node.js.

pnpm должен ставиться способом, актуальным на момент реализации. Нужно учитывать, что pnpm может требовать Node.js, если не используется standalone-вариант или `@pnpm/exe`.

Bun должен быть опциональным компонентом. Нужно учитывать, что Bun распространяется как single executable и может устанавливаться разными официальными способами.

### 7.3 `base`

Можно добавить как отдельный профиль.

Назначение: минимальная безопасная подготовка сервера без Docker.

Состав:

```text
Базовые пакеты
Системные обновления
Firewall, если выбран
Hardening, если выбран
Swap, если выбран
Deploy user, если выбран
```

---

## 8. Модули

CLI должен быть модульным. Каждый модуль должен уметь:

```text
1. Проверить текущее состояние.
2. Сказать, нужен ли action.
3. Выполнить action.
4. Проверить результат.
5. Вернуть понятный статус.
```

Каждый модуль должен быть idempotent: повторный запуск не должен ломать сервер и не должен выполнять лишние действия без необходимости.

---

## 9. Обязательные модули v1

### 9.1 OS detection

Должен определять:

```text
distribution
version
codename
architecture
package manager
systemd availability
current user
root/sudo status
```

Если ОС не поддерживается — инструмент должен завершиться с понятной ошибкой.

### 9.2 Doctor

Команда:

```text
serverctl doctor
```

Проверяет:

```text
ОС
архитектуру
наличие sudo/root
доступ к apt
доступ к интернету
DNS
systemd
диск
память
открытые/занятые порты
установленный Docker
установленный Caddy
firewall status
ssh config hints
```

Doctor не должен менять систему.

### 9.3 Docker

Состав:

```text
Docker Engine
Docker CLI
containerd
Docker Compose plugin
проверка docker service
проверка docker compose
опциональное добавление deploy user в docker group
```

Требования:

```text
1. Использовать актуальные официальные инструкции Docker.
2. Не использовать устаревшие команды из старых гайдов.
3. Не ставить Docker Desktop.
4. Не ставить Docker через snap.
5. Не ставить docker-compose v1.
6. Уметь проверять уже установленный Docker.
7. Не ломать существующую установку.
```

Docker Desktop не является целью этого CLI. Нужно устанавливать именно Docker Engine/server-side tooling.

### 9.4 Caddy

Caddy должен быть опциональным.

Варианты:

```text
1. Не устанавливать Caddy.
2. Установить Caddy на host.
3. Только проверить, что Caddy уже есть.
```

Важно: в некоторых проектах Caddy может быть внутри репозитория или Docker Compose. Поэтому CLI не должен навязывать host-level Caddy.

Модуль Caddy v1 не должен генерировать Caddyfile для конкретного проекта. Максимум:

```text
установка
проверка сервиса
проверка версии
опциональная базовая валидация config path
```

### 9.5 Firewall

Firewall должен быть опциональным.

Для v1 основной backend:

```text
ufw
```

Модуль должен уметь:

```text
1. Проверить установлен ли ufw.
2. Установить ufw, если выбран.
3. Разрешить SSH-порт.
4. Разрешить 80/443, если выбран web-сценарий или Caddy.
5. Включить firewall только после safety-check.
6. Не заблокировать текущую SSH-сессию.
```

Критически важно: перед включением firewall нужно убедиться, что текущий SSH-порт разрешён.

### 9.6 Swap

Swap должен быть опциональным.

Модуль должен уметь:

```text
1. Проверить текущий swap.
2. Создать swapfile заданного размера.
3. Не создавать второй swap, если уже есть подходящий.
4. Добавить persistent mount.
5. Проверить результат.
```

Размер по умолчанию можно предложить:

```text
2G
```

Но пользователь должен иметь возможность изменить размер или отключить модуль.

### 9.7 Deploy user

Опциональный модуль.

Должен уметь:

```text
1. Создать пользователя.
2. Добавить SSH key.
3. Добавить пользователя в нужные группы.
4. Настроить sudo, если выбрано.
5. Не перетирать существующие authorized_keys без подтверждения.
```

Нельзя автоматически отключать root login или password auth, пока не проверено, что новый пользователь реально может подключиться. Так как CLI запускается внутри сервера, он не может гарантированно проверить внешний SSH login без дополнительной логики. Поэтому dangerous hardening должен идти отдельным подтверждаемым шагом.

### 9.8 Security hardening

Hardening должен быть разбит на независимые опции.

Опции:

```text
disable root SSH login
disable password authentication
install/configure fail2ban
enable unattended upgrades
basic sysctl hardening
restrict SSH users
```

Каждый пункт должен быть:

```text
optional
объяснён в wizard
виден в dry-run
отдельно подтверждаем
```

Особенно опасные действия:

```text
disable root SSH login
disable password SSH auth
change SSH port
```

Их нельзя применять молча в составе профиля.

### 9.9 Node tooling

Опциональный модуль только для профиля `node` или при явном выборе.

Состав:

```text
nvm
Node.js LTS/current, по выбору пользователя
pnpm
bun
```

Требования:

```text
1. Не ставить Node/Bun/pnpm по умолчанию в docker-only.
2. Устанавливать tooling под выбранного пользователя, а не хаотично под root.
3. Объяснить пользователю, что для Docker-only серверов host-level Node tooling обычно не нужен.
4. Использовать актуальные официальные инструкции.
```

---

## 10. Конфигурация

Формат:

```text
YAML
```

CLI должен поддерживать:

```text
serverctl init
serverctl apply --config server.yml
serverctl validate --config server.yml
serverctl plan --config server.yml
```

Конфиг должен хранить:

```text
profile
target OS constraints
modules
module options
user settings
firewall rules
hardening choices
runtime choices
```

Не нужно хранить секреты в конфиге.

---

## 11. Команды CLI

Минимальный набор:

```text
serverctl doctor
serverctl init
serverctl validate
serverctl plan
serverctl apply
serverctl status
serverctl version
```

Желательный набор:

```text
serverctl module list
serverctl module status <name>
serverctl logs
```

Команды установки отдельных модулей можно добавить позже, но архитектура должна это позволять:

```text
serverctl install docker
serverctl install caddy
serverctl setup firewall
serverctl setup swap
```

---

## 12. Execution plan

Перед применением CLI должен строить план:

```text
Step 1: update apt cache
Step 2: install required packages
Step 3: add Docker repository
Step 4: install Docker Engine
Step 5: enable Docker service
...
```

Но команды внутри плана должны формироваться актуальной реализацией, а не хардкодиться в ТЗ.

План должен делить шаги по статусам:

```text
will run
already ok
will skip
needs confirmation
dangerous
unsupported
failed precondition
```

---

## 13. Idempotency

Ключевое требование: инструмент можно запускать повторно.

Каждый модуль должен проверять текущее состояние перед изменениями.

Примеры ожидаемого поведения:

```text
Docker уже установлен -> проверить версию и service status, не переустанавливать без необходимости
Caddy уже установлен -> не перетирать конфиги
ufw уже включён -> не сбрасывать правила
swap уже есть -> не создавать новый swapfile
deploy user уже есть -> не пересоздавать
authorized_keys уже есть -> не перетирать
```

---

## 14. Логирование

CLI должен писать:

```text
человеческий summary в stdout
подробный лог в файл
```

Рекомендуемый путь:

```text
/var/log/serverctl/
```

Лог должен содержать:

```text
timestamp
command
profile
config path
OS info
executed steps
exit codes
errors
```

Не логировать секреты.

---

## 15. Ошибки и recovery

Ошибки должны быть понятными.

Плохой вариант:

```text
command failed exit status 1
```

Хороший вариант:

```text
Docker repository was not added successfully.
Likely causes:
- unsupported Debian/Ubuntu codename
- network/DNS problem
- GPG key issue
Suggested next steps:
- run serverctl doctor
- check /var/log/serverctl/...
```

CLI не обязан делать полноценный rollback в v1, но должен давать rollback hints.

---

## 16. Безопасность

Запрещено:

```text
1. Молча отключать SSH password auth.
2. Молча отключать root login.
3. Молча менять SSH port.
4. Перетирать существующие конфиги Caddy/SSH/ufw.
5. Устанавливать произвольные скрипты без явного источника и проверки.
6. Использовать неофициальные инструкции, если есть официальные.
```

Все внешние installation methods должны быть получены из официальных docs проекта на момент реализации:

```text
Docker -> Docker official docs
Caddy -> Caddy official docs
Bun -> Bun official docs
pnpm -> pnpm official docs
nvm -> nvm official GitHub
```

---

## 17. Open-source требования

Проект должен иметь:

```text
README
LICENSE
CONTRIBUTING, можно позже
docs/
examples/
GitHub Actions
GitHub Releases
install.sh
checks/tests
```

README должен объяснять:

```text
что делает инструмент
что он не делает
поддерживаемые ОС
быстрый старт
профили
dry-run
безопасность
как удалить
```

---

## 18. Что НЕ входит в v1

Не делать в первой версии:

```text
remote SSH orchestration
замену Ansible
Terraform/OpenTofu integration
генерацию docker-compose.yml для проектов
генерацию Caddyfile под конкретные приложения
управление доменами
управление DNS
backup-систему
мониторинг
TUI
плагины
поддержку Fedora/Arch/Alpine
поддержку cloud-init
```

Это важно, чтобы MVP не разросся.

---

## 19. Критерии готовности v1

v1 считается готовой, если:

```text
1. CLI собирается в single binary.
2. Работает на Ubuntu 22.04/24.04 и Debian stable.
3. Есть install.sh.
4. Есть doctor.
5. Есть init wizard.
6. Есть apply по YAML config.
7. Есть dry-run/plan.
8. Профиль docker-only работает.
9. Профиль node работает.
10. Caddy можно включить/выключить.
11. Firewall можно включить/выключить.
12. Hardening опционален.
13. Повторный запуск не ломает систему.
14. Есть README и docs.
15. Есть базовые тесты.
```

---

## 20. Рекомендуемый MVP-порядок реализации

### Этап 1 — каркас CLI

```text
CLI framework
version command
doctor skeleton
config schema
logging
OS detection
```

### Этап 2 — планировщик

```text
module interface
execution plan
dry-run
status model
step runner
```

### Этап 3 — docker-only profile

```text
apt prerequisites
Docker official repository flow
Docker Engine
Docker Compose plugin
Docker service checks
```

### Этап 4 — optional modules

```text
ufw
swap
deploy user
basic hardening
```

### Этап 5 — node profile

```text
nvm
Node.js
pnpm
bun
user-level install semantics
```

### Этап 6 — Caddy

```text
optional host-level Caddy install
service check
config safety checks
```

### Этап 7 — polish/open-source

```text
README
install.sh
GitHub Actions
GitHub Releases
examples
test matrix
```

---

## 21. Источники, которые агент обязан перепроверить перед реализацией

Перед написанием installation logic агент должен открыть и перепроверить актуальные официальные инструкции:

```text
Docker Engine Ubuntu/Debian official docs
Docker Compose plugin official docs
Caddy install official docs
Bun installation official docs
pnpm installation official docs
nvm official GitHub README
Ubuntu/Debian release support status
```

Нельзя копировать команды из старых блогов и ответов StackOverflow, если есть официальная документация.

---

## 22. Итоговое решение по концепции

Финальная концепция:

```text
Open-source Go CLI для repeatable setup Ubuntu/Debian VPS.
Запускается внутри сервера.
Имеет wizard, YAML config, dry-run, doctor, profiles.
Основные профили: base, docker-only, node.
Caddy, firewall, hardening, swap, deploy user — опциональные модули.
Bootstrap script только устанавливает CLI, но не настраивает сервер.
```

Главный принцип:

```text
Безопасный, понятный, idempotent server setup tool, а не магический bash-скрипт.
```
