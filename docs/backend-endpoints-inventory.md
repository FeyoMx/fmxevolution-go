# Backend Endpoints Inventory

Fecha de inspeccion: 2026-05-21.

Alcance inspeccionado:

- API actual: `cmd/api/main.go` + `internal/server/server.go`.
- Router legacy: `cmd/evolution-go/main.go` + `pkg/routes/routes.go` + `pkg/core/c0.go`.
- Middlewares de auth/tenant/rate limit: `internal/middleware/*`, `pkg/middleware/auth_middleware.go`.
- Handlers de instancia, mensajes, webhooks, compatibilidad y runtime.

## Hallazgos clave

- `POST /instance/setPresence/:instanceName` existe ahora en `cmd/api` como ruta de compatibilidad para `n8n-nodes-evolution-api`.
- Siguen sin existir estas variantes exactas: `/instance/:id/presence`, `/instance/set-presence/:instanceName`, `/instance/:instanceName/setPresence`, `/chat/setPresence/:instanceName`, `/message/setPresence/:instanceName`.
- `POST /message/presence/:instanceName` tambien existe ahora en `cmd/api`; online/offline se mapea a `alwaysOnline`, mientras `composing`, `paused` y `recording` devuelven `501 unsupported_chat_presence`.
- En la API actual `cmd/api`, el equivalente para estado online/conexion de instancia es el grupo lifecycle/status: `POST /instance/:id/connect`, `GET /instance/:id/status`, `GET /instance/:id/runtime`, `PUT /instance/:id/advanced-settings` con `alwaysOnline`.
- La API actual usa `:id` como referencia flexible en muchas rutas de instancia: acepta ID de instancia o nombre via `Service.resolve`. Las rutas `/instance/id/:instanceID/...` son alias estrictos por ID. Las rutas compat heredadas usan `:instanceName`.
- El router legacy standalone usa `apikey` de instancia para casi todas las rutas funcionales y no toma `instanceName` en la URL; la instancia se resuelve desde el token. Sus rutas admin usan `:instanceId`.

## Auth y middlewares

API actual (`internal/server`):

- Publicas: health/readiness, `POST /auth/login`, `POST /auth/refresh`, `POST /tenant`.
- Protegidas: `Authorization: Bearer <access_token>` o `X-API-Key: <tenant_api_key>` o `apikey: <tenant_api_key>`.
- Tenant guard: valida tenant autenticado; opcionalmente verifica `X-Tenant-ID` y `X-Tenant-Slug` si se envian.
- Roles: `owner`, `admin`, `agent`, segun ruta.
- Rate limits: search en chat/messages, broadcast, webhook inbound/outbound.

Router legacy (`pkg/routes`):

- Admin legacy: header `apikey: <GLOBAL_API_KEY>`.
- Funcional legacy: header `apikey: <INSTANCE_API_KEY>`; el middleware carga la instancia en contexto.
- Gate de licencia en `cmd/evolution-go`: bloquea rutas no exentas si la licencia no esta activa.

## Inventario API actual (`cmd/api`)

| Metodo | Ruta | Handler/Archivo | Estado | Auth requerida | Body esperado | Notas |
|---|---|---|---|---|---|---|
| GET | `/healthz` | inline `internal/server/server.go` | Actual | No | No | Health probe. |
| GET | `/livez` | inline `internal/server/server.go` | Actual | No | No | Liveness. |
| GET | `/readyz` | inline `internal/server/server.go` | Actual | No | No | Hace `db.PingContext`. |
| POST | `/auth/login` | `authHandler.Login` `internal/auth/handler.go` | Actual | No | `{ tenant_slug|tenant, email, password }` JSON/form/query/header | Devuelve JWT/refresh. |
| POST | `/auth/refresh` | `authHandler.Refresh` `internal/auth/handler.go` | Actual | No | `{ refresh_token|refreshToken }` JSON/form/query | Renueva tokens. |
| POST | `/tenant` | `tenantHandler.Create` `internal/tenant/handler.go` | Actual | No | `{ name, slug, admin_name, admin_email, admin_password }` | Crea tenant y admin. |
| GET | `/auth/me` | `authHandler.Me` `internal/auth/handler.go` | Actual | Bearer/API key | No | Roles owner/admin/agent por auth general. |
| POST | `/auth/logout` | `authHandler.Logout` `internal/auth/handler.go` | Actual | Bearer/API key | No | Acknowledgement stateless. |
| GET | `/dashboard/metrics` | `dashboardHandler.Metrics` `internal/dashboard/handler.go` | Actual | owner/admin/agent | No | Tenant scoped. |
| GET | `/tenant` | `tenantHandler.Get` `internal/tenant/handler.go` | Actual | owner/admin/agent | No | Tenant actual. |
| GET | `/ai/settings` | `aiHandler.GetTenantSettings` `internal/ai/handler.go` | Actual | owner/admin/agent | No | Config AI tenant. |
| PUT | `/ai/settings` | `aiHandler.ConfigureTenant` `internal/ai/handler.go` | Actual | owner/admin | `{ enabled, auto_reply, provider, model, base_url, system_prompt }` | Tenant scoped. |
| GET | `/ai/instances/:instanceID` | `aiHandler.GetInstanceSettings` `internal/ai/handler.go` | Actual | owner/admin/agent | No | Usa `instanceID`. |
| PUT | `/ai/instances/:instanceID` | `aiHandler.ConfigureInstance` `internal/ai/handler.go` | Actual | owner/admin | `{ enabled, auto_reply }` | Usa `instanceID`. |
| POST | `/instance` | `instanceHandler.Create` `internal/instance/handler.go` | Actual | owner/admin | `{ name, engine_instance_id?, webhook_url? }` o aliases `instanceName`, `instance`, `engineInstanceId`, `webhookUrl` | Crea instancia tenant. |
| GET | `/instance` | `instanceHandler.List` `internal/instance/handler.go` | Actual | owner/admin/agent | No | Lista tenant. |
| POST | `/instance/setPresence/:instanceName` | `LegacySetPresence` `internal/instance/compat_handler.go` | Compat n8n | owner/admin/agent | `{ presence }`, `{ state }` o `{ alwaysOnline }` | `available/online/true` -> `alwaysOnline=true`; `unavailable/offline/false` -> `false`; chat states devuelven `501 unsupported_chat_presence`. |
| GET | `/instance/:id` | `instanceHandler.Get` `internal/instance/handler.go` | Actual | owner/admin/agent | No | `:id` debe ser ID para este handler. |
| GET | `/instance/:id/settings` | `instanceHandler.Settings` `internal/instance/handler.go` | Actual | owner/admin/agent | No | `:id` es referencia flexible ID/nombre. |
| GET | `/instance/:id/advanced-settings` | `instanceHandler.GetAdvancedSettings` `internal/instance/handler.go` | Actual | owner/admin/agent | No | `:id` flexible ID/nombre. |
| PUT | `/instance/:id/advanced-settings` | `instanceHandler.UpdateAdvancedSettings` `internal/instance/handler.go` | Actual | owner/admin | `{ alwaysOnline, rejectCall, msgRejectCall, readMessages, ignoreGroups, ignoreStatus }` | Aqui vive `alwaysOnline`. |
| POST | `/instance/:id/messages/text` | `instanceHandler.SendText` `internal/instance/handler.go` | Actual | owner/admin/agent | `{ number, text, delay? }` | Encola envio; `:id` flexible. |
| GET | `/instance/:id/messages/text/:jobID` | `instanceHandler.SendTextJobStatus` `internal/instance/handler.go` | Actual | owner/admin/agent | No | Consulta job de texto. |
| POST | `/instance/:id/chats/search` | `instanceHandler.SearchChats` `internal/instance/integration_handler.go` | Actual | owner/admin/agent + rate limit | `{ where: { remoteJid?, query?|search? } }` | Live/cache. |
| POST | `/instance/:id/messages/search` | `instanceHandler.SearchMessages` `internal/instance/integration_handler.go` | Actual | owner/admin/agent + rate limit | `{ where:{ key:{ remoteJid? }, query? }, limit?, cursor? }` | Requiere remote JID. |
| POST | `/instance/:id/messages/media` | `instanceHandler.SendMediaMessage` `internal/instance/integration_handler.go` | Actual | owner/admin/agent | `{ number, type|mediatype, mimetype?, caption?, media? base64, url?, fileName?, delay?, options? }` | Imagen/video/audio/document. |
| POST | `/instance/:id/messages/audio` | `instanceHandler.SendAudioMessage` `internal/instance/integration_handler.go` | Actual | owner/admin/agent | `{ number, audio|audioMessage.audio base64, delay?, options? }` | Audio WhatsApp/PTT. |
| GET | `/instance/:id/websocket` | `instanceHandler.GetWebsocketConfig` `internal/instance/integration_handler.go` | Actual parcial | owner/admin/agent | No | Config connector. |
| PUT | `/instance/:id/websocket` | `instanceHandler.SetWebsocketConfig` `internal/instance/integration_handler.go` | Actual parcial | owner/admin | `{ enabled, events }` o envelope `{ websocket:{ enabled, events } }` | Runtime legacy requerido. |
| GET | `/instance/:id/rabbitmq` | `instanceHandler.GetRabbitMQConfig` `internal/instance/integration_handler.go` | Actual parcial | owner/admin/agent | No | Config connector. |
| PUT | `/instance/:id/rabbitmq` | `instanceHandler.SetRabbitMQConfig` `internal/instance/integration_handler.go` | Actual parcial | owner/admin | `{ enabled, events }` o envelope `{ rabbitmq:{ enabled, events } }` | Runtime legacy requerido. |
| GET | `/instance/:id/sqs` | `instanceHandler.GetSQSConfig` `internal/instance/integration_handler.go` | Stub | owner/admin/agent | No | Devuelve feature no soportada. |
| PUT | `/instance/:id/sqs` | `instanceHandler.SetSQSConfig` `internal/instance/integration_handler.go` | Stub | owner/admin | `{ enabled, events }` | Devuelve feature no soportada. |
| GET | `/instance/:id/proxy` | `instanceHandler.GetProxyConfig` `internal/instance/integration_handler.go` | Actual parcial | owner/admin/agent | No | SOCKS5 solamente. |
| PUT | `/instance/:id/proxy` | `instanceHandler.SetProxyConfig` `internal/instance/integration_handler.go` | Actual parcial | owner/admin | `{ enabled, host, port, protocol?, username?, password? }` | Solo `socks5`. |
| GET | `/instance/:id/chatwoot` | `instanceHandler.GetChatwootConfig` `internal/instance/integration_handler.go` | Stub | owner/admin/agent | No | Devuelve feature no soportada. |
| PUT | `/instance/:id/chatwoot` | `instanceHandler.SetChatwootConfig` `internal/instance/integration_handler.go` | Stub | owner/admin | Config JSON | Devuelve feature no soportada. |
| GET | `/instance/:id/openai` | `ListOpenAIResources` `internal/instance/integration_handler.go` | Stub | owner/admin/agent | No | Feature no soportada. |
| POST | `/instance/:id/openai` | `CreateOpenAIResource` `internal/instance/integration_handler.go` | Stub | owner/admin | JSON libre | Feature no soportada. |
| GET | `/instance/:id/openai/settings` | `GetOpenAISettings` `internal/instance/integration_handler.go` | Stub | owner/admin/agent | No | Feature no soportada. |
| PUT | `/instance/:id/openai/settings` | `UpdateOpenAISettings` `internal/instance/integration_handler.go` | Stub | owner/admin | JSON libre | Feature no soportada. |
| POST | `/instance/:id/openai/status` | `ChangeOpenAIStatus` `internal/instance/integration_handler.go` | Stub | owner/admin | JSON libre | Feature no soportada. |
| GET | `/instance/:id/openai/:resourceId` | `GetOpenAIResource` `internal/instance/integration_handler.go` | Stub | owner/admin/agent | No | Feature no soportada. |
| PUT | `/instance/:id/openai/:resourceId` | `UpdateOpenAIResource` `internal/instance/integration_handler.go` | Stub | owner/admin | JSON libre | Feature no soportada. |
| DELETE | `/instance/:id/openai/:resourceId` | `DeleteOpenAIResource` `internal/instance/integration_handler.go` | Stub | owner/admin | No | Feature no soportada. |
| GET | `/instance/:id/openai/:resourceId/sessions` | `ListOpenAISessions` `internal/instance/integration_handler.go` | Stub | owner/admin/agent | No | Feature no soportada. |
| GET/POST/PUT/DELETE | `/instance/:id/{typebot,dify,n8n,evoai,evolutionBot,flowise}...` | resource handlers `internal/instance/integration_handler.go` | Stub/parcial | owner/admin(/agent for GET) | JSON segun accion | Mismos patrones que OpenAI: list/create/settings/status/resource/sessions. Mayormente feature no soportada o parcial. |
| GET | `/instance/id/:instanceID` | `instanceHandler.GetByID` `internal/instance/handler.go` | Actual alias | owner/admin/agent | No | `instanceID` estricto. |
| GET | `/instance/id/:instanceID/advanced-settings` | `GetAdvancedSettings` `internal/instance/handler.go` | Actual alias | owner/admin/agent | No | `instanceID` estricto. |
| PUT | `/instance/id/:instanceID/advanced-settings` | `UpdateAdvancedSettings` `internal/instance/handler.go` | Actual alias | owner/admin | `{ alwaysOnline, rejectCall, msgRejectCall, readMessages, ignoreGroups, ignoreStatus }` | `instanceID` estricto. |
| POST | `/instance/id/:instanceID/messages/text` | `SendText` `internal/instance/handler.go` | Actual alias | owner/admin/agent | `{ number, text, delay? }` | `instanceID` estricto. |
| GET | `/instance/id/:instanceID/messages/text/:jobID` | `SendTextJobStatus` `internal/instance/handler.go` | Actual alias | owner/admin/agent | No | `instanceID` estricto. |
| POST | `/instance/id/:instanceID/chats/search` | `SearchChats` `internal/instance/integration_handler.go` | Actual alias | owner/admin/agent + rate limit | `{ where: { remoteJid?, query? } }` | `instanceID` estricto. |
| POST | `/instance/id/:instanceID/messages/search` | `SearchMessages` `internal/instance/integration_handler.go` | Actual alias | owner/admin/agent + rate limit | Message search JSON | `instanceID` estricto. |
| GET | `/group/fetchAllGroups/:instanceName` | `LegacyFetchAllGroups` `internal/instance/group_handler.go` | Compat | owner/admin/agent | No | Usa `instanceName`. |
| GET | `/v2/group/findGroup/:instanceName` | `LegacyFindGroup` `internal/instance/group_handler.go` | Compat | owner/admin/agent | Query/body no requerido | Usa `instanceName`. |
| GET | `/v2/group/fetchAllGroups/:instanceName` | `LegacyFetchAllGroups` `internal/instance/group_handler.go` | Compat | owner/admin/agent | No | Usa `instanceName`. |
| POST | `/chat/findChats/:instanceName` | `LegacyFindChats` `internal/instance/compat_handler.go` | Compat | owner/admin/agent + rate limit | `{ where: { remoteJid?, query? } }` | Usa `instanceName`. |
| POST | `/chat/findMessages/:instanceName` | `LegacyFindMessages` `internal/instance/compat_handler.go` | Compat | owner/admin/agent + rate limit | Message search JSON | Usa `instanceName`. |
| POST | `/message/presence/:instanceName` | `LegacyChatPresence` `internal/instance/compat_handler.go` | Compat n8n | owner/admin/agent | `{ presence }`, `{ state }` o `{ alwaysOnline }` | Mismo comportamiento que `/instance/setPresence/:instanceName`. |
| POST | `/message/markread/:instanceName` | `LegacyMarkRead` `internal/instance/compat_handler.go` | Compat n8n registrado | owner/admin/agent | `{ number, id }` | Devuelve `501` compatible con `success=false`, `error=unsupported_markread`. |
| POST | `/message/sendText/:instanceName` | `LegacySendText` `internal/instance/compat_handler.go` | Compat n8n | owner/admin/agent | `{ number, text, delay? }` | Responde envelope Evolution `{ success, message, data }`. |
| POST | `/message/sendMedia/:instanceName` | `LegacySendMedia` `internal/instance/compat_handler.go` | Compat n8n | owner/admin/agent | Media JSON con `caption?` | Responde envelope Evolution `{ success, message, data }`. |
| POST | `/message/sendWhatsAppAudio/:instanceName` | `LegacySendAudio` `internal/instance/compat_handler.go` | Compat n8n | owner/admin/agent | Audio JSON | Responde envelope Evolution `{ success, message, data }`. |
| POST | `/instance/:id/connect` | `Connect` `internal/instance/handler.go` | Actual | owner/admin | No | `:id` flexible ID/nombre. |
| POST | `/instance/id/:instanceID/connect` | `ConnectByID` `internal/instance/handler.go` | Actual alias | owner/admin | No | `instanceID` estricto. |
| POST | `/instance/:id/disconnect` | `Disconnect` `internal/instance/handler.go` | Actual | owner/admin | No | `:id` flexible ID/nombre. |
| POST | `/instance/id/:instanceID/disconnect` | `DisconnectByID` `internal/instance/handler.go` | Actual alias | owner/admin | No | `instanceID` estricto. |
| POST | `/instance/:id/reconnect` | `Reconnect` `internal/instance/handler.go` | Actual | owner/admin | No | Runtime bridge requerido. |
| POST | `/instance/id/:instanceID/reconnect` | `ReconnectByID` `internal/instance/handler.go` | Actual alias | owner/admin | No | `instanceID` estricto. |
| POST | `/instance/:id/pair` | `Pair` `internal/instance/handler.go` | Actual | owner/admin | `{ phone }` o `{ number }` | Pairing code. |
| POST | `/instance/id/:instanceID/pair` | `PairByID` `internal/instance/handler.go` | Actual alias | owner/admin | `{ phone }` o `{ number }` | `instanceID` estricto. |
| DELETE | `/instance/:id/logout` | `Logout` `internal/instance/handler.go` | Actual | owner/admin | No | Runtime bridge requerido. |
| DELETE | `/instance/id/:instanceID/logout` | `LogoutByID` `internal/instance/handler.go` | Actual alias | owner/admin | No | `instanceID` estricto. |
| GET | `/instance/:id/status` | `Status` `internal/instance/handler.go` | Actual | owner/admin/agent | No | Status normalizado. |
| GET | `/instance/id/:instanceID/status` | `StatusByID` `internal/instance/handler.go` | Actual alias | owner/admin/agent | No | `instanceID` estricto. |
| GET | `/instance/:id/runtime` | `RuntimeStatus` `internal/instance/handler.go` | Actual | owner/admin/agent | No | Durable + live bridge. |
| GET | `/instance/id/:instanceID/runtime` | `RuntimeStatusByID` `internal/instance/handler.go` | Actual alias | owner/admin/agent | No | `instanceID` estricto. |
| GET | `/instance/:id/runtime/history` | `RuntimeHistory` `internal/instance/handler.go` | Actual | owner/admin/agent | Query `limit?` | Historial durable. |
| GET | `/instance/id/:instanceID/runtime/history` | `RuntimeHistoryByID` `internal/instance/handler.go` | Actual alias | owner/admin/agent | Query `limit?` | `instanceID` estricto. |
| POST | `/instance/:id/history/backfill` | `BackfillHistory` `internal/instance/handler.go` | Actual | owner/admin | `{ chat_jid|chat|remote_jid, message_id?, timestamp?, is_from_me?, is_group?, count? }` | Solicita history sync. |
| POST | `/instance/id/:instanceID/history/backfill` | `BackfillHistoryByID` `internal/instance/handler.go` | Actual alias | owner/admin | Backfill JSON | `instanceID` estricto. |
| GET | `/instance/:id/qr` | `QRCode` `internal/instance/handler.go` | Actual | owner/admin/agent | No | QR actual o fallback 202. |
| GET | `/instance/:id/qrcode` | `QRCode` `internal/instance/handler.go` | Actual alias | owner/admin/agent | No | Alias QR. |
| GET | `/instance/id/:instanceID/qr` | `QRCodeByID` `internal/instance/handler.go` | Actual alias | owner/admin/agent | No | `instanceID` estricto. |
| GET | `/instance/id/:instanceID/qrcode` | `QRCodeByID` `internal/instance/handler.go` | Actual alias | owner/admin/agent | No | `instanceID` estricto. |
| DELETE | `/instance` | `Delete` `internal/instance/handler.go` | Actual | owner/admin | Query `id` | Borra instancia. |
| DELETE | `/instance/:id` | `Delete` `internal/instance/handler.go` | Actual | owner/admin | No | `:id` de ruta. |
| DELETE | `/instance/id/:instanceID` | `DeleteByID` `internal/instance/handler.go` | Actual alias | owner/admin | No | `instanceID` estricto. |
| GET | `/contacts` | `crmHandler.ListContacts` `internal/crm/handler.go` | Actual | owner/admin/agent | Query filtros/paginacion si servicio los usa | CRM. |
| GET | `/contacts/:id` | `crmHandler.GetContact` `internal/crm/handler.go` | Actual | owner/admin/agent | No | CRM. |
| POST | `/contacts` | `crmHandler.CreateContact` `internal/crm/handler.go` | Actual | owner/admin/agent | `{ phone, name?, email?, tags?, metadata? }` | CRM. |
| PATCH | `/contacts/:id` | `crmHandler.UpdateContact` `internal/crm/handler.go` | Actual | owner/admin/agent | `{ phone?, name?, email?, tags?, metadata? }` | CRM. |
| POST | `/contacts/:id/notes` | `crmHandler.AddNote` `internal/crm/handler.go` | Actual | owner/admin/agent | `{ body|note }` | CRM. |
| POST | `/contacts/:id/tags` | `crmHandler.AssignTags` `internal/crm/handler.go` | Actual | owner/admin/agent | `{ tags: [] }` | CRM. |
| GET | `/broadcast` | `broadcastHandler.List` `internal/broadcast/handler.go` | Actual | owner/admin/agent + rate limit | Query `limit?` | Tenant scoped. |
| POST | `/broadcast` | `broadcastHandler.Create` `internal/broadcast/handler.go` | Actual | owner/admin/agent + rate limit | `{ name, instance_id, recipients, message, scheduled_at? }` | Broadcast. |
| GET | `/broadcast/:id` | `broadcastHandler.Get` `internal/broadcast/handler.go` | Actual | owner/admin/agent + rate limit | No | Broadcast detail. |
| GET | `/broadcast/:id/recipients` | `broadcastHandler.ListRecipients` `internal/broadcast/handler.go` | Actual | owner/admin/agent + rate limit | Query `page?`, `limit?`, `status?`, `query?` | Paginated recipients. |
| GET | `/webhook` | `webhookHandler.List` `internal/webhook/handler.go` | Actual + compat | owner/admin/agent | Optional query `instanceName` | Sin query lista endpoints; con `instanceName` lee webhook legacy de instancia. |
| POST | `/webhook` | `webhookHandler.Create` `internal/webhook/handler.go` | Actual + compat | owner/admin | Actual `{ name, url, events? }`; legacy `{ instanceName|instance|instance_id, webhook_url|webhookUrl|url, enabled?, events?, webhook? }` | Payload legacy actualiza webhook de instancia. |
| GET | `/webhook/:id` | `webhookHandler.Get` `internal/webhook/handler.go` | Actual | owner/admin/agent | No | Endpoint registry. |
| POST | `/webhook/inbound` | `webhookHandler.DispatchInbound` `internal/webhook/handler.go` | Actual | owner/admin/agent + rate limit | `{ event, instance_id?, instanceName?, data|payload }` | Dispatch inbound tenant. |
| POST | `/webhook/outbound` | `webhookHandler.DispatchOutbound` `internal/webhook/handler.go` | Actual | owner/admin/agent + rate limit | `{ event, instance_id?, instanceName?, data|payload }` | Dispatch outbound tenant. |

## Inventario router legacy standalone (`cmd/evolution-go`)

Estas rutas existen si se ejecuta `cmd/evolution-go`, no el binario `cmd/api`.

| Metodo | Ruta | Handler/Archivo | Estado | Auth requerida | Body esperado | Notas |
|---|---|---|---|---|---|---|
| GET | `/swagger/*any` | Swagger `pkg/routes/routes.go` | Legacy public | No | No | UI docs. |
| GET | `/favicon.ico` | inline `pkg/routes/routes.go` | Legacy public | No | No | 204. |
| GET | `/manager/*any` | static manager `pkg/routes/routes.go` | Legacy public | No | No | React manager. |
| GET | `/manager` | static manager `pkg/routes/routes.go` | Legacy public | No | No | React manager. |
| GET | `/server/ok` | `serverHandler.ServerOk` `pkg/server/handler` | Legacy public | No | No | Health. |
| GET | `/license/status` | inline `pkg/core/c0.go` | Legacy public | No | No | License state. |
| GET | `/license/register` | inline `pkg/core/c0.go` | Legacy public | No | Query `redirect_uri?` | Inicia registro licencia. |
| GET | `/license/activate` | inline `pkg/core/c0.go` | Legacy public | No | Query `code` | Activa licencia. |
| GET | `/ws` | inline `cmd/evolution-go/main.go` | Legacy public token | Query `token=<GLOBAL_API_KEY>&instanceId=<id>` | No | Websocket events. |
| POST | `/instance/create` | `instanceHandler.Create` `pkg/instance/handler` | Legacy admin | `apikey: GLOBAL_API_KEY` | `{ name, token?, webhook?, events?, ... }` | Crea instancia legacy. |
| GET | `/instance/all` | `instanceHandler.All` `pkg/instance/handler` | Legacy admin | `apikey: GLOBAL_API_KEY` | No | Lista instancias legacy. |
| GET | `/instance/info/:instanceId` | `instanceHandler.Info` `pkg/instance/handler` | Legacy admin | `apikey: GLOBAL_API_KEY` | No | Usa `instanceId`. |
| DELETE | `/instance/delete/:instanceId` | `instanceHandler.Delete` `pkg/instance/handler` | Legacy admin | `apikey: GLOBAL_API_KEY` | No | Usa `instanceId`. |
| POST | `/instance/proxy/:instanceId` | `instanceHandler.SetProxy` `pkg/instance/handler` | Legacy admin | `apikey: GLOBAL_API_KEY` | `{ host, port, username?, password? }` | Usa `instanceId`. |
| DELETE | `/instance/proxy/:instanceId` | `instanceHandler.DeleteProxy` `pkg/instance/handler` | Legacy admin | `apikey: GLOBAL_API_KEY` | No | Usa `instanceId`. |
| POST | `/instance/forcereconnect/:instanceId` | `instanceHandler.ForceReconnect` `pkg/instance/handler` | Legacy admin | `apikey: GLOBAL_API_KEY` | No | Usa `instanceId`. |
| GET | `/instance/logs/:instanceId` | `instanceHandler.GetLogs` `pkg/instance/handler` | Legacy admin | `apikey: GLOBAL_API_KEY` | Query log options | Usa `instanceId`. |
| POST | `/instance/connect` | `instanceHandler.Connect` `pkg/instance/handler` | Legacy | `apikey: INSTANCE_API_KEY` | No/body opcional | Instancia desde apikey. |
| GET | `/instance/status` | `instanceHandler.Status` `pkg/instance/handler` | Legacy | `apikey: INSTANCE_API_KEY` | No | Instancia desde apikey. |
| GET | `/instance/qr` | `instanceHandler.Qr` `pkg/instance/handler` | Legacy | `apikey: INSTANCE_API_KEY` | No | QR. |
| POST | `/instance/pair` | `instanceHandler.Pair` `pkg/instance/handler` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number }` | Valida number. |
| POST | `/instance/disconnect` | `instanceHandler.Disconnect` `pkg/instance/handler` | Legacy | `apikey: INSTANCE_API_KEY` | No | Instancia desde apikey. |
| POST | `/instance/reconnect` | `instanceHandler.Reconnect` `pkg/instance/handler` | Legacy | `apikey: INSTANCE_API_KEY` | No | Instancia desde apikey. |
| DELETE | `/instance/logout` | `instanceHandler.Logout` `pkg/instance/handler` | Legacy | `apikey: INSTANCE_API_KEY` | No | Instancia desde apikey. |
| GET | `/instance/:instanceId/advanced-settings` | `GetAdvancedSettings` `pkg/instance/handler` | Legacy | `apikey: INSTANCE_API_KEY` | No | Usa `instanceId` en ruta. |
| PUT | `/instance/:instanceId/advanced-settings` | `UpdateAdvancedSettings` `pkg/instance/handler` | Legacy | `apikey: INSTANCE_API_KEY` | `{ alwaysOnline, rejectCall, msgRejectCall, readMessages, ignoreGroups, ignoreStatus }` | Usa `instanceId`. |
| POST | `/send/text` | `sendHandler.SendText` `pkg/sendMessage/handler` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number, text, delay?, quoted? }` | Envia texto. |
| POST | `/send/link` | `sendHandler.SendLink` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number, text?, link, title?, description?, thumbnail? }` | Envia link. |
| POST | `/send/media` | `sendHandler.SendMedia` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number, mediatype/type, mimetype?, caption?, media|url, fileName? }` | Envia media. |
| POST | `/send/poll` | `sendHandler.SendPoll` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number, name, options, selectableCount? }` | Enquete. |
| POST | `/send/sticker` | `sendHandler.SendSticker` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number, sticker|media|url }` | Sticker. |
| POST | `/send/location` | `sendHandler.SendLocation` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number, latitude, longitude, name?, address? }` | Location. |
| POST | `/send/contact` | `sendHandler.SendContact` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number, contact }` | Valida contato. |
| POST | `/send/button` | `sendHandler.SendButton` | Legacy | `apikey: INSTANCE_API_KEY` | Button JSON | Pode depender de soporte WA. |
| POST | `/send/list` | `sendHandler.SendList` | Legacy | `apikey: INSTANCE_API_KEY` | List JSON | Pode depender de soporte WA. |
| POST | `/user/info` | `userHandler.GetUser` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number }` | Info usuario. |
| POST | `/user/check` | `userHandler.CheckUser` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number }` | Verifica WA. |
| POST | `/user/avatar` | `userHandler.GetAvatar` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number }` | Avatar. |
| GET | `/user/contacts` | `userHandler.GetContacts` | Legacy | `apikey: INSTANCE_API_KEY` | No | Contactos. |
| GET | `/user/privacy` | `userHandler.GetPrivacy` | Legacy | `apikey: INSTANCE_API_KEY` | No | Privacy. |
| POST | `/user/privacy` | `userHandler.SetPrivacy` | Legacy | `apikey: INSTANCE_API_KEY` | Privacy JSON | Ajusta privacy. |
| POST | `/user/block` | `userHandler.BlockContact` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number }` | Bloquea. |
| POST | `/user/unblock` | `userHandler.UnblockContact` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number }` | Desbloquea. |
| GET | `/user/blocklist` | `userHandler.GetBlockList` | Legacy | `apikey: INSTANCE_API_KEY` | No | Lista bloqueados. |
| POST | `/user/profilePicture` | `userHandler.SetProfilePicture` | Legacy | `apikey: INSTANCE_API_KEY` | `{ image|picture }` | Perfil. |
| POST | `/user/profileName` | `userHandler.SetProfileName` | Legacy | `apikey: INSTANCE_API_KEY` | `{ name }` | Perfil. |
| POST | `/user/profileStatus` | `userHandler.SetProfileStatus` | Legacy | `apikey: INSTANCE_API_KEY` | `{ status }` | Perfil. |
| POST | `/message/react` | `messageHandler.React` `pkg/message/handler` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number, id, reaction, fromMe?, participant? }` | Reaccion. |
| POST | `/message/presence` | `messageHandler.ChatPresence` `pkg/message/handler` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number, state }` | Unico endpoint de presencia de chat. |
| POST | `/message/markread` | `messageHandler.MarkRead` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number, id: [] }` | Marcar leido. |
| POST | `/message/downloadmedia` | `messageHandler.DownloadMedia` | Legacy | `apikey: INSTANCE_API_KEY` | Media key/message JSON | Descargar media. |
| POST | `/message/status` | `messageHandler.GetMessageStatus` | Legacy | `apikey: INSTANCE_API_KEY` | `{ id }` | Status mensaje. |
| POST | `/message/delete` | `messageHandler.DeleteMessageEveryone` | Legacy | `apikey: INSTANCE_API_KEY` | `{ chat, messageId }` | Delete everyone. |
| POST | `/message/edit` | `messageHandler.EditMessage` | Legacy | `apikey: INSTANCE_API_KEY` | `{ chat, messageId, message }` | Edita texto. |
| POST | `/chat/pin` | `chatHandler.ChatPin` | Legacy TODO | `apikey: INSTANCE_API_KEY` | `{ number }` | Comentado "not working". |
| POST | `/chat/unpin` | `chatHandler.ChatUnpin` | Legacy TODO | `apikey: INSTANCE_API_KEY` | `{ number }` | Comentado "not working". |
| POST | `/chat/archive` | `chatHandler.ChatArchive` | Legacy TODO | `apikey: INSTANCE_API_KEY` | `{ number }` | Comentado "not working". |
| POST | `/chat/unarchive` | `chatHandler.ChatUnarchive` | Legacy TODO | `apikey: INSTANCE_API_KEY` | `{ number }` | Comentado "not working". |
| POST | `/chat/mute` | `chatHandler.ChatMute` | Legacy TODO | `apikey: INSTANCE_API_KEY` | `{ number }` | Comentado "not working". |
| POST | `/chat/unmute` | `chatHandler.ChatUnmute` | Legacy TODO | `apikey: INSTANCE_API_KEY` | `{ number }` | Comentado "not working". |
| POST | `/chat/history-sync` | `chatHandler.HistorySyncRequest` | Legacy | `apikey: INSTANCE_API_KEY` | `{ jid/chat, count? }` | Solicita sync. |
| GET | `/group/list` | `groupHandler.ListGroups` | Legacy | `apikey: INSTANCE_API_KEY` | No | Grupos. |
| POST | `/group/info` | `groupHandler.GetGroupInfo` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number }` | Info grupo. |
| POST | `/group/invitelink` | `groupHandler.GetGroupInviteLink` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number }` | Link. |
| POST | `/group/photo` | `groupHandler.SetGroupPhoto` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number, image }` | Foto. |
| POST | `/group/name` | `groupHandler.SetGroupName` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number, name }` | Nombre. |
| POST | `/group/description` | `groupHandler.SetGroupDescription` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number, description }` | Descripcion. |
| POST | `/group/create` | `groupHandler.CreateGroup` | Legacy | `apikey: INSTANCE_API_KEY` | `{ name, participants: [] }` | Crear grupo. |
| POST | `/group/participant` | `groupHandler.UpdateParticipant` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number, participants, action }` | Participantes. |
| GET | `/group/myall` | `groupHandler.GetMyGroups` | Legacy TODO | `apikey: INSTANCE_API_KEY` | No | Comentado "not working". |
| POST | `/group/join` | `groupHandler.JoinGroupLink` | Legacy | `apikey: INSTANCE_API_KEY` | `{ link }` | Join. |
| POST | `/group/leave` | `groupHandler.LeaveGroup` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number }` | Leave. |
| POST | `/call/reject` | `callHandler.RejectCall` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number, callId? }` | Rechaza llamada. |
| POST | `/community/create` | `communityHandler.CreateCommunity` | Legacy | `apikey: INSTANCE_API_KEY` | `{ name }` | Comunidad. |
| POST | `/community/add` | `communityHandler.CommunityAdd` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number, communityId }` | Comunidad. |
| POST | `/community/remove` | `communityHandler.CommunityRemove` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number, communityId }` | Comunidad. |
| POST | `/label/chat` | `labelHandler.ChatLabel` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number, labelId }` | Etiqueta chat. |
| POST | `/label/message` | `labelHandler.MessageLabel` | Legacy | `apikey: INSTANCE_API_KEY` | `{ messageId, labelId }` | Etiqueta mensaje. |
| POST | `/label/edit` | `labelHandler.EditLabel` | Legacy | `apikey: INSTANCE_API_KEY` | `{ id, name?, color? }` | Edita label. |
| GET | `/label/list` | `labelHandler.GetLabels` | Legacy | `apikey: INSTANCE_API_KEY` | No | Lista labels. |
| POST | `/unlabel/chat` | `labelHandler.ChatUnlabel` | Legacy | `apikey: INSTANCE_API_KEY` | `{ number, labelId }` | Quita etiqueta. |
| POST | `/unlabel/message` | `labelHandler.MessageUnlabel` | Legacy | `apikey: INSTANCE_API_KEY` | `{ messageId, labelId }` | Quita etiqueta. |
| POST | `/newsletter/create` | `newsletterHandler.CreateNewsletter` | Legacy | `apikey: INSTANCE_API_KEY` | `{ name, description?, picture? }` | Newsletter. |
| GET | `/newsletter/list` | `newsletterHandler.ListNewsletter` | Legacy | `apikey: INSTANCE_API_KEY` | No | Newsletter. |
| POST | `/newsletter/info` | `newsletterHandler.GetNewsletter` | Legacy | `apikey: INSTANCE_API_KEY` | `{ newsletterId }` | Newsletter. |
| POST | `/newsletter/link` | `newsletterHandler.GetNewsletterInvite` | Legacy | `apikey: INSTANCE_API_KEY` | `{ newsletterId }` | Newsletter. |
| POST | `/newsletter/subscribe` | `newsletterHandler.SubscribeNewsletter` | Legacy | `apikey: INSTANCE_API_KEY` | `{ newsletterId }` | Newsletter. |
| POST | `/newsletter/messages` | `newsletterHandler.GetNewsletterMessages` | Legacy | `apikey: INSTANCE_API_KEY` | `{ newsletterId }` | Newsletter. |
| GET | `/polls/:pollMessageId/results` | `pollHandler.GetPollResults` | Legacy | `apikey: INSTANCE_API_KEY` | No | Resultado encuesta. |

## Presencia: verificacion de variantes solicitadas

| Variante | Existe | Donde | Parametro usado | Recomendacion |
|---|---:|---|---|---|
| `POST /instance/setPresence/:instanceName` | Si | `cmd/api` | `instanceName` flexible por tenant | Usar desde n8n para online/offline. |
| `/instance/:id/presence` | No | Ningun router | N/A | No usar. |
| `/instance/set-presence/:instanceName` | No | Ningun router | N/A | No usar. |
| `/instance/:instanceName/setPresence` | No | Ningun router | N/A | No usar. |
| `/chat/setPresence/:instanceName` | No | Ningun router | N/A | No usar. |
| `/message/setPresence/:instanceName` | No | Ningun router | N/A | No usar. |
| `POST /message/presence/:instanceName` | Si | `cmd/api` | `instanceName` flexible por tenant | Alias compatible para online/offline; chat presence devuelve `501`. |
| `POST /message/presence` | Si | Router legacy `pkg/routes` | Instancia desde `apikey` | Usar solo en binario legacy para presencia de chat. |
| `PUT /instance/:id/advanced-settings` | Si | API actual | `id` flexible ID/nombre | Usar para `alwaysOnline`. |
| `GET /instance/:id/runtime` | Si | API actual | `id` flexible ID/nombre | Usar para saber online/live/durable. |
| `GET /instance/:id/status` | Si | API actual | `id` flexible ID/nombre | Usar para estado simple. |

## URLs recomendadas para n8n

Reemplazos:

- `{{$env.API_BASE_URL}}`: base URL del backend, por ejemplo `https://api.tu-dominio.com`.
- `{{$json.instance_id}}`: ID de instancia SaaS.
- `{{$json.instanceName}}`: nombre de instancia, solo cuando uses rutas compat por nombre.
- `{{$env.API_TOKEN}}`: JWT o API key tenant para `cmd/api`.
- `{{$env.INSTANCE_API_KEY}}`: apikey de instancia para el router legacy `cmd/evolution-go`.

Headers para API actual:

```http
Authorization: Bearer {{$env.API_TOKEN}}
Content-Type: application/json
```

Tambien sirve:

```http
X-API-Key: {{$env.API_TOKEN}}
Content-Type: application/json
```

Headers para router legacy:

```http
apikey: {{$env.INSTANCE_API_KEY}}
Content-Type: application/json
```

### Instancia en API actual

| Operacion n8n | Metodo | URL | Body |
|---|---|---|---|
| Crear instancia | POST | `{{$env.API_BASE_URL}}/instance` | `{ "name": "ventas", "webhook_url": "https://n8n.example/webhook/wa" }` |
| Listar instancias | GET | `{{$env.API_BASE_URL}}/instance` | No |
| Ver instancia por ID | GET | `{{$env.API_BASE_URL}}/instance/id/{{$json.instance_id}}` | No |
| Ver instancia por referencia flexible | GET | `{{$env.API_BASE_URL}}/instance/{{$json.instance_id}}/settings` | No |
| Conectar | POST | `{{$env.API_BASE_URL}}/instance/{{$json.instance_id}}/connect` | No |
| Reconectar | POST | `{{$env.API_BASE_URL}}/instance/{{$json.instance_id}}/reconnect` | No |
| Desconectar | POST | `{{$env.API_BASE_URL}}/instance/{{$json.instance_id}}/disconnect` | No |
| Logout | DELETE | `{{$env.API_BASE_URL}}/instance/{{$json.instance_id}}/logout` | No |
| Pairing code | POST | `{{$env.API_BASE_URL}}/instance/{{$json.instance_id}}/pair` | `{ "phone": "5215512345678" }` |
| QR | GET | `{{$env.API_BASE_URL}}/instance/{{$json.instance_id}}/qrcode` | No |
| Estado simple | GET | `{{$env.API_BASE_URL}}/instance/{{$json.instance_id}}/status` | No |
| Estado runtime recomendado | GET | `{{$env.API_BASE_URL}}/instance/{{$json.instance_id}}/runtime` | No |
| Historial runtime | GET | `{{$env.API_BASE_URL}}/instance/{{$json.instance_id}}/runtime/history?limit=50` | No |
| Activar always online | PUT | `{{$env.API_BASE_URL}}/instance/{{$json.instance_id}}/advanced-settings` | `{ "alwaysOnline": true, "rejectCall": false, "readMessages": true, "ignoreGroups": false, "ignoreStatus": false }` |
| Borrar instancia | DELETE | `{{$env.API_BASE_URL}}/instance/id/{{$json.instance_id}}` | No |

### Mensajes en API actual

| Operacion n8n | Metodo | URL | Body |
|---|---|---|---|
| Enviar texto recomendado | POST | `{{$env.API_BASE_URL}}/instance/{{$json.instance_id}}/messages/text` | `{ "number": "5215512345678", "text": "Hola desde n8n", "delay": 0 }` |
| Consultar job texto | GET | `{{$env.API_BASE_URL}}/instance/{{$json.instance_id}}/messages/text/{{$json.job_id}}` | No |
| Enviar media por URL | POST | `{{$env.API_BASE_URL}}/instance/{{$json.instance_id}}/messages/media` | `{ "number": "5215512345678", "type": "image", "url": "https://example.com/image.jpg", "caption": "Imagen desde n8n" }` |
| Enviar media base64 | POST | `{{$env.API_BASE_URL}}/instance/{{$json.instance_id}}/messages/media` | `{ "number": "5215512345678", "type": "document", "mimetype": "application/pdf", "fileName": "file.pdf", "media": "<base64>" }` |
| Enviar audio WhatsApp | POST | `{{$env.API_BASE_URL}}/instance/{{$json.instance_id}}/messages/audio` | `{ "number": "5215512345678", "audio": "<base64-ogg>" }` |
| Buscar chats | POST | `{{$env.API_BASE_URL}}/instance/{{$json.instance_id}}/chats/search` | `{ "where": { "query": "Luis" } }` |
| Buscar mensajes | POST | `{{$env.API_BASE_URL}}/instance/{{$json.instance_id}}/messages/search` | `{ "where": { "key": { "remoteJid": "5215512345678@s.whatsapp.net" } }, "limit": 50 }` |
| Compat enviar texto por nombre | POST | `{{$env.API_BASE_URL}}/message/sendText/{{$json.instanceName}}` | `{ "number": "5215512345678", "text": "Hola compat" }` |
| Compat enviar media por nombre | POST | `{{$env.API_BASE_URL}}/message/sendMedia/{{$json.instanceName}}` | `{ "number": "5215512345678", "type": "image", "url": "https://example.com/image.jpg" }` |
| Compat audio por nombre | POST | `{{$env.API_BASE_URL}}/message/sendWhatsAppAudio/{{$json.instanceName}}` | `{ "number": "5215512345678", "audio": "<base64-ogg>" }` |

### Webhook en API actual

| Operacion n8n | Metodo | URL | Body |
|---|---|---|---|
| Listar endpoints webhook | GET | `{{$env.API_BASE_URL}}/webhook` | No |
| Crear endpoint webhook tenant | POST | `{{$env.API_BASE_URL}}/webhook` | `{ "name": "n8n inbound", "url": "https://n8n.example/webhook/wa", "events": ["MESSAGE", "CONNECTION"] }` |
| Ver endpoint webhook | GET | `{{$env.API_BASE_URL}}/webhook/{{$json.webhook_id}}` | No |
| Leer webhook legacy por instancia | GET | `{{$env.API_BASE_URL}}/webhook?instanceName={{$json.instanceName}}` | No |
| Actualizar webhook legacy de instancia | POST | `{{$env.API_BASE_URL}}/webhook` | `{ "instanceName": "ventas", "webhook_url": "https://n8n.example/webhook/wa", "enabled": true, "events": ["MESSAGE", "CONNECTION"], "webhook": { "base64": false, "byEvents": true } }` |
| Dispatch inbound manual | POST | `{{$env.API_BASE_URL}}/webhook/inbound` | `{ "event": "MESSAGE", "instance_id": "{{$json.instance_id}}", "payload": { "text": "test" } }` |
| Dispatch outbound manual | POST | `{{$env.API_BASE_URL}}/webhook/outbound` | `{ "event": "MESSAGE", "instance_id": "{{$json.instance_id}}", "payload": { "text": "test" } }` |

### Presencia y online

| Necesidad | Metodo | URL | Headers | Body | Nota |
|---|---|---|---|---|---|
| Saber si instancia esta online en API actual | GET | `{{$env.API_BASE_URL}}/instance/{{$json.instance_id}}/runtime` | Bearer/API key tenant | No | Recomendado; devuelve `durable`, `live`, `connected`, `logged_in`. |
| Saber status simple en API actual | GET | `{{$env.API_BASE_URL}}/instance/{{$json.instance_id}}/status` | Bearer/API key tenant | No | Menos detallado que runtime. |
| Mantener always online en API actual | PUT | `{{$env.API_BASE_URL}}/instance/{{$json.instance_id}}/advanced-settings` | Bearer/API key tenant | `{ "alwaysOnline": true, "rejectCall": false, "readMessages": true, "ignoreGroups": false, "ignoreStatus": false }` | Equivalente operativo para presencia/online persistente. |
| SetPresence compatible n8n | POST | `{{$env.API_BASE_URL}}/instance/setPresence/{{$json.instanceName}}` | Bearer/API key tenant | `{ "presence": "available" }` | Mapea a `alwaysOnline=true`. |
| Presence compatible n8n | POST | `{{$env.API_BASE_URL}}/message/presence/{{$json.instanceName}}` | Bearer/API key tenant | `{ "state": "unavailable" }` | Mapea a `alwaysOnline=false`. |
| Setear presencia de chat en router legacy | POST | `{{$env.API_BASE_URL}}/message/presence` | `apikey: {{$env.INSTANCE_API_KEY}}` | `{ "number": "5215512345678", "state": "composing" }` | Solo `cmd/evolution-go`; no lleva `instanceName` en URL. |

Estados comunes para `state` en presencia legacy dependen de whatsmeow/WA; los usados normalmente son `composing`, `paused`, `recording` y/o `available` segun soporte del servicio.

## Recomendaciones

1. Para n8n sobre la API actual, usar siempre rutas `POST/GET /instance/:id/...` con el ID de instancia SaaS y `Authorization: Bearer` o `X-API-Key`.
2. Configurar los nodos Evolution API de n8n contra `POST /instance/setPresence/:instanceName` para online/offline; esa ruta ya existe en `cmd/api`.
3. Si necesitas presencia de chat real (`composing`, `recording`), hoy solo esta en el router legacy `POST /message/presence`; en `cmd/api`, las rutas compat devuelven `501 unsupported_chat_presence`.
4. Para estado online de instancia en `cmd/api`, usar `GET /instance/:id/runtime` y `PUT /instance/:id/advanced-settings` con `alwaysOnline: true`.
5. Preferir aliases `/instance/id/:instanceID/...` cuando el workflow tenga el UUID/ID real; usar `/instance/:id/...` solo si se necesita compatibilidad con nombre.
