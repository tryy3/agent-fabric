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
| **unsupported model** | En model i en connections model catalog som planen saknar provider adapter för (dess wire API). Visas och flaggas på connection men erbjuds aldrig för val. Beräknas när connection levereras, lagras inte. |
| **model specs** | Referensdata om providers och models (capabilities, limits, modalities, priser per miljon tokens), synkad från en specs source. Skiljer sig från model catalog, som listar vad en connection kan leverera. |
| **specs source** | URL:en som planen synkar model specs från; models.dev som standard, eller valfri URL med samma `api.json`-struktur. |
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
| **publish** | Att skicka ett Projects files till en publish provider så att de nås via en länk. Varje lyckad publish sparas i Projectets publish history. Download är en export, inte en publish. |
| **publish provider** | En destination som hostar publicerade project files, exempelvis Netlify. Ett Project kan publicera till flera providers, till exempel en intern och en publik. |
| **latest link** | En publish providers länk som alltid visar dess live deploy (för Netlify site-URL:en). |
| **deploy link** | En länk fäst vid en enskild publish som fortsätter visa den versionen efter senare publishes. |
| **publish history** | Ett Projects sparade publishes, nyast först, var och en med provider, latest link och deploy link (`GET /v1/projects/{id}/publishes`). |
| **live deploy** | Den deploy som ett publish provider-mål just nu visar via sin latest link. Oftast den senaste publishen; efter en rollback hos providern kan det vara en äldre. Den kan inte tas bort. |
| **publish sync** | Att stämma av ett Projects publish history mot varje publish provider som kan lista sina deploys: importerar deploys gjorda på annat håll, tar bort deploys som raderats hos providern (manuellt eller av dess retention-städning) och uppdaterar länkar och live deploy (`POST /v1/projects/{id}/publishes/sync`). |
| **delete publish** | Att radera en deploy hos dess publish provider så att dess deploy link slutar fungera, och sedan ta bort den ur publish history (`DELETE /v1/projects/{id}/publishes/{publishId}`). Live deploy nekas. |
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
| **permission rule** | En användarinställning som ger tool calls av ett verktyg som matchar ett mönster en fast risk score (1-10), som en inbyggd regel; permission mode gör poängen till kör, fråga eller avbryt. Settings redigerar den som verktyg, mönster, score och en valfri brytare "Ask the scorers" (`consult`) som låter gate-scorers justera poängen. API:t tar fortfarande emot de äldre actions `allow` (score 1), `ask` och `deny` (score 10), som gäller i alla permission modes. Sätts per assistant och för hela planet. |
| **rule tier** | En inbyggd klass i gatens regler (skrivskyddade kommandon, destruktiva kommandon, förbjudna program, ...) med ett grundbetyg, en åtgärd och för vissa en programlista. Listas av `GET /v1/permissions/builtins` och kan åsidosättas i `permissions.builtins`; en **settled** tier frågar aldrig en gate scorer. |
| **gate cascade** | Gatens nivåer i ordning: regler, sedan en valfri snabb nivå, sedan en valfri djup nivå som bara tillfrågas när de tidigare lämnar ett anrop oavgjort. |
| **gate scorer** | En valfri modellnivå i gate cascade som betygsätter ett tool calls risk: den **snabba nivån** (en System One-beslutsmodell; Jev rekommenderas) eller den **djupa nivån** (en chattmodell, som får sänka ett betyg med ett begränsat antal steg). |
| **gate instructions** | Vanlig text som användaren skriver om vad agenten får och inte får göra (till exempel "pusha aldrig till main"). Ett standardvärde för hela planet med valfri åsidosättning per assistant, som instructions, men läst av gate scorers (och läsbart för agenten via ett verktyg) och aldrig del av effective instructions. Bara katalogen redigerar dem. Ett nytt plan börjar med en standardtext. De pinnas vid `session/new`. |
| **gate question** | En ja/nej-fråga som en gate scorer besvarar om ett tool call (till exempel "raderar kommandot data oåterkalleligt?"). Frågorna grupperas i en frågeuppsättning per verktygsfamilj och användaren kan redigera dem. En fråga är en **tripwire** eller en **policy question**. |
| **tripwire** | En gate question om anropets natur (destruktivt, läcker credentials) snarare än användarens regler. Ett ja sätter ett lägsta risk score som ingen senare nivå får sänka, oavsett gate instructions. Utöver de inbyggda kan användaren skriva egna (en **custom question**), alltid en tripwire. |
| **policy question** | En gate question som bara ställs när gate instructions finns: om anropet bryter mot dem, och om de uttryckligen tillåter just detta anrop. Ett uttryckligt tillstånd upphäver bara överträdelsen, aldrig en tripwire. |
| **gate cooldown** | En väntetid som dubblas för varje anrop gaten nekat eller avbrutit i en session och avtar med tystnad, innan gaten bedömer nästa anrop; under den ser modellen `denied: cooling down`. En ny prompt från användaren nollställer den. |
| **session taint** | Tillståndet för en session som har läst webbinnehåll; gaten frågar då från risk 5 och den djupa nivån får inte sänka betyg. |
| **risk band** | Ett namngivet intervall av risk scores: safe (1-2), low (3-4), elevated (5-6), high (7-8), cancel (9-10). |
| **permission mode** | Hur stor del av gatens riskskala som körs utan att fråga: `ask` (Ask for approval), `auto_approve` (Approve for me), `auto` (Run automatically) eller `full` (Full access). En assistant sparar läget som dess sessioner startar i (`settings.permissions.mode`, aldrig `full`); en levande session kan byta med ACP `session/set_mode`, bara för den sessionen. |
| **Full access** | Permission mode `full`: frågar aldrig och avbryter bara från sitt avbrytgränsvärde (risk 10 om inte `permissions.policies.full.cancelAt` ändrar det). Gäller bara en session: väljs i composern, bekräftas av användaren varje gång (`_meta.confirmFullAccess` på `session/set_mode`), sparas aldrig på en assistant och återställs aldrig i en ny session. |
| **permission policy** | Gränsvärdena för fråga och avbryt, i risk score, för ett permission mode. Inbyggda (`gate.DefaultPolicies`) och ändras för hela planet i `permissions.policies`. |
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
| **skill** | En mapp med en `SKILL.md` (frontmatter och instruktioner) och valfria extra filer som agenten läser vid behov. Namn och beskrivning visas för modellen; innehållet läses först när skillen används. En skill är inte ett tool: det är instruktioner som agenten följer med sina vanliga tools. |
| **skill source** | Lagret en skill kommer från: `builtin` (inbäddad i planet, skrivskyddad), `plane`, `project` (`.agent-fabric/skills/` eller `.agents/skills/` i projektets filer) eller `assistant`. En skill med samma namn i ett senare lager ersätter den under. |
| **skill toggle** | Namnlistan `skills.disabled` i plane settings, project settings eller assistant settings; en skill som är avstängd på någon nivå är avstängd. Nya skills är på som standard. |
| **load_skill** | Stabilt control-plane-tool som returnerar en pinnad skills SKILL.md, eller en extra fil ur dess mapp när en path anges. Skrivskyddat, risk 1 i gate, och bara erbjudet till modellen när sessionen har minst en aktiverad skill. |
| **skill invocation** | En prompt som börjar med `/name` för en pinnad skill. Planet tolkar skillens deklarerade flaggor (`--depth`, `--rounds`, `--time`, `--save`), lägger skillens instruktioner som kontext för den prompten, skickar resten som frågan och sparar anropet (name, source, hash, flags) som en `skill`-part i turen. Ett okänt `/name` skickas oförändrat som vanlig text. |
| **tool round** | Ett steg i en Prompt där modellen ber om tools och får resultaten tillbaka. En vanlig Prompt tillåter 8 som standard. |
| **run limits** | Budgeten för en Prompt i tool rounds och väggklocktid: `limits.defaultToolRounds` (en vanlig Prompt), `limits.maxToolRounds` (taket för alla körningar, skills inräknade) och `limits.maxTurnSeconds`, satta för hela planet och per assistant (`settings.limits`, aldrig över planets maximum) och pinnade vid `session/new`. När en körning har använt sina rounds eller sin tid får modellen en sista tur utan tools för att skriva sitt bästa svar. |

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
