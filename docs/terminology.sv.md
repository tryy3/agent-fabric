# Projektterminologi

English version: [Project terminology](terminology.md).

Det här är Agent Fabrics kanoniska ordlista. Behåll de engelska termerna i kod, API, UI och båda språkversionerna. Kvalificera agent, session, role, prompt, context, resource och memory när betydelsen inte redan är tydlig.

## Produkt och runtime

| Term | Betydelse |
| --- | --- |
| **Assistant** | Konfigurerbart Catalog-objekt som äger identity, purpose, instructions, inference-val, tools, policy och execution settings. |
| **Assistant purpose** | Mänskligt läsbar förklaring av varför en Assistant finns och vilket ansvar den har. |
| **assistant configuration** | Inställningarna som ingår i en Assistant; inte ett separat objekt. |
| **ACP Agent** | Agent-rollen i ACP. Control plane exponerar varje Assistant som en logisk ACP Agent. |
| **agent runtime** | Implementationen i control plane som driver prompt- och tool-loopen för en ACP Agent. |
| **Project** | Catalog-ägd avgränsning som grupperar threads, Assistants, settings, resources, environments och files. |
| **Workbench** | Client-UI:t för ett öppet Project: files, chat, editors, previews, dock och panels. |

Använd **Assistant** för det konfigurerade produktobjektet. Använd Agent endast i en kvalificerad teknisk term, exempelvis **ACP Agent** eller **agent runtime**.

## Conversation

| Term | Betydelse |
| --- | --- |
| **ACP connection** | En initialiserad transportanslutning mellan ACP Client och ACP Agent. |
| **ACP session** | Ett aktivt runtime-handtag skapat av `session/new`; det fäster en snapshot av en Assistant och current model. |
| **thread** | Den beständiga och användarsynliga konversationen. En thread kan överleva många ACP sessions. |
| **turn** | En user prompt och allt efterföljande arbete fram till completion, cancellation eller failure. |
| **attempt** | En assistant-svarvariant för en user prompt inom en turn. Soft-supersede-retry behåller tidigare attempts för inspektion; endast den **aktiva** attempten visas på huvudvägen i conversation view. |
| **attempt status** | Livscykel för en assistant attempt: `running` (pågående), `completed` (lyckad terminal), `failed` (fel eller interrupted efter omstart), eller `cancelled` (användarstopp via `session/cancel`). Model context hydrerar aktiva assistants endast när status är `completed`. |
| **round** | En intern iteration i agent runtime med ett model-anrop och dess svar. |
| **provider request** | Ett konkret anrop till en inference service och dess svar. Inspector använder termen; round används för runtime tracing. |
| **thread record** | Den kanoniska, serverägda posten för en thread. |
| **thread message** | En beständig user- eller assistant-post i en thread record. |
| **message content** | Den huvudsakliga synliga texten i en thread message. |
| **message part** | En beständig strukturerad del, exempelvis **sent** (pinad effective instructions för sessionen), thought, tool call, message, usage eller **error** (terminal feldetalj för en misslyckad attempt). |
| **turn activity** | Client-presentation som härleds från message parts; inte en separat source of truth. |
| **thread history** | Den ordnade följden av beständiga thread messages och parts. |
| **conversation view** | Workbenchs projektion av thread history som visas för användaren. |
| **model context** | Semantisk input som byggs för ett model-anrop; den kan skilja sig från thread history och conversation view. |
| **provider message** | En message serialiserad i ett provider wire API för ett provider request. |
| **in-turn tool context** | Tool calls och results som behålls under aktuell turn; de behöver inte återhydreras i nästa turn. |
| **provider request capture** | Den oföränderliga och rensade posten av vad som skickades till och togs emot från en inference service. |

```text
thread record
  └─ thread history
      └─ thread messages
          ├─ message content
          └─ message parts ──rendered as──> turn activities in conversation view

thread history + current input + runtime data
  └─ model context
      └─ provider messages
          └─ provider request capture
```

## Instructions, context och memory

| Term | Betydelse |
| --- | --- |
| **Platform instructions** | Delad Agent Fabric-vägledning för hur assistants använder tools, arbetar i miljön och hanterar uppgifter. Gäller varje assistant. |
| **Assistant instructions** | Beständiga Assistant-ägda instruktioner som definierar roll, expertis, prioriteringar och kommunikationsstil. |
| **Runtime context** | Sessionsspecifika fakta från plattformen (datum, tidszon, modell, workspace). Redigerbar som plan-bred mall; variabler resolvas när effective instructions pinnas. |
| **instruction variable** | En `{{name}}`-placeholder i valfri instruktionskälla, som ersätts när effective instructions pinnas (till exempel `{{currentDate}}`, `{{timezone}}`, `{{workspaceRoot}}`, `{{modelId}}`). |
| **instruction override** | En uttrycklig och avgränsad ändring av en instruktionskälla, exempelvis för en thread eller turn. |
| **effective instructions** | Det provider-oberoende resultatet av att sammanställa Platform instructions, Assistant instructions, Runtime context och tillämpliga overrides för ett model-anrop, efter att instruction variables ersatts. |
| **provider instruction message** | Den provider-specifika serialiseringen av effective instructions. |
| **system prompt** | Informell synonym för **effective instructions**. Det är alltid det första innehållet modellen ser; bara placeringen i anropet skiljer sig mellan adaptrar (Anthropic-fältet `system`, Responses-fältet `instructions`, Chat Completions inledande **system message**). Föredra "effective instructions" i kod, API:er och UI. |
| **system message** | Ett provider instruction message med message role `system`. Endast Chat Completions-adaptern skickar ett för effective instructions. |
| **developer message** | Ett provider instruction message med message role `developer`. |
| **user prompt** | Användarens aktuella förfrågan som startar eller fortsätter en turn. |
| **prompt template** | En återanvändbar, eventuellt parametriserad mall från vilken en prompt kan skapas. |
| **MCP prompt** | En prompt eller prompt template som exponeras av en MCP server. |
| **policy** | En regel som control plane verkställer. Säkerhet får inte bero enbart på model instructions. |
| **message role** | Den semantiska rollen för en message, exempelvis `user`, `assistant`, `system`, `developer` eller `tool`. |
| **protocol role** | En parts ansvar i ett protokoll, exempelvis ACP Client eller ACP Agent. |
| **access role** | En behörighets- eller organisationsroll. Använd alltid den kvalificerade termen. |
| **context source** | En källa som kan bidra till model context, exempelvis instructions, history, memory eller en resource. |
| **memory** | Beständig serverägd kunskap som kan hämtas för framtida model contexts. Thread history och client UI state är inte memory. |
| **memory record** | En beständig post i memory. |
| **memory retrieval** | Valet av relevanta memory records för den aktuella förfrågan. |
| **memory injection** | Införandet av hämtad memory i model context. |
| **MCP resource** | Data som exponeras av en MCP server. Den kan vara en context source men är inte ett tool eller memory. |
| **Catalog resource** | Project-relaterat material som lagras eller refereras genom Catalog. |
| **client UI state** | Lokal presentationsdata som öppna panels, valda files och Workbench-layout. En **WorkbenchStateStore** sparar den. |

```text
Platform instructions ──┐
Assistant instructions ─┼──> (variable substitution) ──> effective instructions ──> provider instruction message
Runtime context ────────┤
instruction overrides ──┘

effective instructions + thread history + user prompt + memory/resources
  └─ model context

policy enforced separately by control plane
```

## Inference

| Term | Betydelse |
| --- | --- |
| **inference connection** | Sparat Catalog-objekt med name, connection type, endpoint, credentials och model catalog. |
| **connection type** | Väljer validering och provider-adapter family för en inference connection. |
| **inference provider** | Den externa leverantören eller produktfamiljen bakom inference. |
| **inference service** | Den konkreta lokala eller externa tjänst som tar emot inference requests. |
| **inference endpoint** | Nätverksadressen till en inference service. |
| **provider adapter** / **streamer** | Kod i control plane som översätter intern data till ett provider wire API. |
| **provider wire API** | Externt request/response-format, exempelvis Chat Completions, Anthropic Messages eller Responses. |
| **model** | Den token-genererande modell som en inference service erbjuder. |
| **model reference** | Lokal metadata som identifierar en model, normalt ID och display name. |
| **model catalog** | Upptäckta model references för en inference connection. |
| **default model** | Den model en Assistant väljer för nya ACP sessions. |
| **current model** | Den model som är fäst i en aktiv ACP session. |
| **inference settings** | Assistant-ägda genereringsinställningar som fästs när en ACP session startar. |

## Files och execution

| Term | Betydelse |
| --- | --- |
| **project files** | Det användarvänliga namnet på ett Projects beständiga files och directories. |
| **project filesystem** | Den tekniska filsystemabstraktionen och dess API för project files. |
| **project path operation** | Flytt (namnbyte), kopiering eller duplicering av en fil eller katalog inom project root. Delas av Workbench-filträdet och senare agentverktyg; skriver aldrig över en befintlig destination. Duplicate kopierar bredvid originalet som `name copy.ext`. |
| **project root** | Rotkatalogen i ett project filesystem. En konkret path kan fortfarande vara `/workspace`. |
| **project volume** | Beständig lagring bakom ett project filesystem. |
| **execution environment** | Den lokala eller containerbaserade runtime-kontext där tools och commands exekveras. |
| **execution backend** | Mekanismen som skapar ett execution environment, exempelvis local, Docker eller Podman. |
| **execution settings** | Catalog-ägda inställningar som ärvs global → Project → Assistant. |
| **execution overlay** | Det internt sammanslagna resultatet av ärvda execution settings. |
| **environment reuse scope** | Hur länge ett execution environment återanvänds: shared, session eller project. |
| **sandbox policy** | Säkerhets- och isoleringsregler som begränsar execution. Sandbox är inte miljöns namn. |
| **control plane configuration** | Processens boot-konfiguration för database, server, storage och tillgängliga execution backends. |
| **`config.json`** | Filen med control plane configuration, inte Project- eller Assistant-ägda execution settings. |

Använd inte workspace som produkt- eller domänterm. Använd **Project**, **Workbench**, **project files** eller **project filesystem**. En konkret `/workspace`-path kan behållas.

## Tools och interactions

| Term | Betydelse |
| --- | --- |
| **tool** | En model-anropbar capability med name, description och input schema. |
| **tool call** | En invocation av ett tool med arguments. |
| **tool result** | Utfallet eller felet från ett tool call. |
| **tool origin** | Exekveraren av ett tool call: `environment`, `control_plane`, `mcp`, `client` eller `provider`. |
| **environment tool** | Ett tool som exekveras genom ett execution environment. |
| **control plane tool** | Ett tool som hanteras direkt av control plane, exempelvis `ask_user`. |
| **MCP tool** | Ett tool som exponeras av en MCP server och anropas genom MCP. |
| **client tool** | Ett tool som exekveras av ansluten client eller device. |
| **provider tool** | En hosted capability som inference provider exekverar. |
| **MCP** | Protokollet genom vilket en MCP server exponerar tools, resources och prompts. MCP är inte ett tool. |
| **MCP server** | Protokollmotparten som exponerar MCP capabilities. |
| **tool policy** | Reglerna som styr vad ett tool får göra. |
| **tool gate** | Komponenten i control plane som bedömer ett tool call som `allow`, `ask` eller `deny`. |
| **risk score** | Ett betyg från 1 till 10 för hur farligt ett tool call är, satt av nivåerna i gate cascade. |
| **permission rule** | En användarinställning som alltid tillåter, frågar om eller nekar tool calls för ett verktyg som matchar ett mönster, i alla permission modes. Sätts per assistant och för hela planet. |
| **rule tier** | En inbyggd klass i gatens regler (skrivskyddade kommandon, destruktiva kommandon, förbjudna program, ...) med ett grundbetyg, en åtgärd och för vissa en programlista. Listas av `GET /v1/permissions/builtins` och kan åsidosättas i `permissions.builtins`; en **settled** tier frågar aldrig en gate scorer. |
| **gate cascade** | Gatens nivåer i ordning: regler, sedan en valfri snabb nivå, sedan en valfri djup nivå som bara tillfrågas när de tidigare lämnar ett anrop oavgjort. |
| **gate scorer** | En valfri modellnivå i gate cascade som betygsätter ett tool calls risk: den **snabba nivån** (en System One-beslutsmodell; Jev rekommenderas) eller den **djupa nivån** (en chattmodell, som får sänka ett betyg med ett begränsat antal steg). |
| **session taint** | Tillståndet för en session som har läst webbinnehåll; gaten frågar då från risk 5 och den djupa nivån får inte sänka betyg. |
| **risk band** | Ett namngivet intervall av risk scores: safe (1-2), low (3-4), elevated (5-6), high (7-8), cancel (9-10). |
| **permission mode** | En inställning per assistant (`ask`, `auto_approve`, `auto`, `full`) som avgör vilka risk scores som körs, frågar eller avbryts. |
| **permission policy** | Gränsvärdena för fråga och avbryt, i risk score, för ett permission mode. |
| **permission request** | En fråga om auktorisation för en planerad handling. |
| **permission decision** | Användarens svar: allow once, allow for this session eller reject. |
| **permission grant** | Åtkomsten som skapas av ett tillåtande permission decision. |
| **grant scope** | Ett permission grants livslängd eller omfattning. |
| **command grant** | Ett permission grant för en session som gäller ett `run_command`-kommandoprefix, till exempel `npm test`; det vidgas aldrig till andra kommandon. |
| **clarification** | En fråga om information eller preferens, inte auktorisation. |
| **elicitation** | ACP-mekanismen som presenterar en strukturerad clarification. |
| **pending interaction** | Internt UI-samlingsnamn för en interaction som väntar på användaren. |
| **hop capture** | En rensad och beständig registrering av trafik mellan delar som ägs av control plane. |
| **integration** | En konfigurerad capability eller anslutning till en extern tjänst. |
| **tool integration** | En catalog-ägd integration som backar en plane tool capability såsom `web_search` eller `fetch_page`. |
| **tool binding** | En Assistant-setting som väljer inherit, disabled eller en specifik tool integration för en capability. |
| **web_search** | Stabilt plane tool som returnerar begränsade publika webbsökresultat. |
| **fetch_page** | Stabilt plane tool som returnerar rengjord Markdown för en publik sida. |

Använd **Allow for this session** för ett grant med ACP-session-scope. Använd **Always** endast för ett persistent grant med uttrycklig livscykel och återkallelse.

## Protocol boundaries

| Gräns | Kanoniskt namn |
| --- | --- |
| Settings och beständig konfiguration | **Catalog HTTP API** (`/v1`) |
| Interaktiv conversation och user interactions | **ACP** (`/acp`) |
| Inference requests och responses | **provider wire API** |
| Externa MCP capabilities | **MCP** |
| Tool dispatch i control plane | **tool registry / execution environment API** |

Ordlistan anger målspråket för [architecture](architecture.md), [decisions](decisions.md), kod, API och produkttext. Implementationsmigreringar spåras i GitHub i stället för att dokumenteras som alternativa namn här.
