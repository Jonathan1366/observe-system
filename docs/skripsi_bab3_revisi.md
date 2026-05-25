# BAB 3
# ANALISIS DAN PERANCANGAN

Bab ini menjelaskan tahapan analisis dan perancangan platform observability berbasis OpenTelemetry yang diintegrasikan dengan Artificial Intelligence. Pembahasan dimulai dari rekayasa kebutuhan, pemilihan metode pengembangan dan teknologi, perancangan proses bisnis (AS-IS dan TO-BE), perancangan sistem menggunakan notasi UML dan ERD, hingga perancangan pengujian.

---

## 3.1 Rekayasa Kebutuhan

### 3.1.1 Deskripsi Objek Penelitian

Penelitian ini tidak dilaksanakan pada perusahaan atau instansi tertentu, melainkan menggunakan lingkungan simulasi (*dummy environment*) yang dirancang khusus oleh peneliti. Lingkungan simulasi ini merepresentasikan sistem backend berbasis *microservices* yang umum dijumpai pada industri perangkat lunak modern, khususnya pada sektor *fintech* dan *trading platform* yang memiliki kompleksitas sistem terdistribusi tinggi (Brechner, 2021).

Lingkungan simulasi terdiri dari tiga sub-sistem yang saling terintegrasi:

1. **Arthur API** — Backend *trading intelligence* berbasis Golang dengan *framework* Chi v5 yang menjadi sumber data telemetri. Sistem ini terdiri dari beberapa *endpoint* yang merepresentasikan layanan microservices: *Market Service*, *Trade Service*, dan *Simulation Service*.

2. **Observe-System** — Platform backend berbasis Golang dengan *framework* Gin v1.12 yang berfungsi sebagai *API gateway* mengintegrasikan data telemetri dari Jaeger dengan kecerdasan buatan dari Google Gemini.

3. **Observe-Flutter** — *Mobile dashboard* berbasis Flutter (Dart SDK ≥3.0) yang menjadi antarmuka pengguna untuk monitoring dan diagnosis *error*.



### 3.1.2 Kebutuhan Fungsional

**Tabel 3.1 Kebutuhan Fungsional Sistem**

| ID | Kebutuhan Fungsional | Prioritas |
|---|---|---|
| FR-01 | Sistem mampu mengumpulkan data *traces* dari backend melalui OpenTelemetry SDK dan OTel Collector | Tinggi |
| FR-02 | Sistem mampu menampilkan daftar *traces* beserta status (ok/error) dan durasi pada *mobile dashboard* | Tinggi |
| FR-03 | Sistem mampu menerima permintaan diagnosis AI dari pengguna terhadap *trace* yang dipilih | Tinggi |
| FR-04 | AI Agent mampu menganalisis data *trace* dan menghasilkan Root Cause Analysis dalam format terstruktur | Tinggi |
| FR-05 | Sistem mampu menampilkan ringkasan kesehatan sistem secara keseluruhan pada halaman *dashboard* | Sedang |
| FR-06 | Sistem mampu memfilter *traces* berdasarkan nama *service* dan status (all/error/ok) | Sedang |
| FR-07 | Sistem menyediakan mekanisme autentikasi berbasis API key untuk mengamankan akses *endpoint* | Sedang |
| FR-08 | AI Agent mampu memberikan *contextual insight* per-section *dashboard* | Sedang |

### 3.1.3 Kebutuhan Non-Fungsional

**Tabel 3.2 Kebutuhan Non-Fungsional Sistem**

| ID | Kategori | Kebutuhan | Metrik Target |
|---|---|---|---|
| NFR-01 | Performance | Response time AI diagnosis | ≤ 30 detik (timeout) |
| NFR-02 | Performance | Response time non-AI endpoint | ≤ 2 detik |
| NFR-03 | Reliability | Sistem tetap operasional saat Jaeger *unreachable* | Availability 99% (graceful fallback) |
| NFR-04 | Interoperability | Format komunikasi antar-komponen | JSON standar + OTLP gRPC |
| NFR-05 | Compatibility | Mobile dashboard | Android API ≥ 33 |
| NFR-06 | Security | Autentikasi akses API | API Key (Bearer / X-API-Key) |
| NFR-07 | Maintainability | Modularitas arsitektur | Separation of concerns |

---

## 3.2 Pemilihan Metode dan Framework

### 3.2.1 Metode Pengembangan Perangkat Lunak

Penelitian ini menggunakan metode pengembangan **iteratif dan inkremental** dengan pendekatan *Software Development Research* (Pressman & Maxim, 2020).

**Tabel 3.3 Tahapan Iterasi Pengembangan**

| Iterasi | Fokus | Output |
|---|---|---|
| 1 | Pembangunan Arthur API + instrumentasi OpenTelemetry | Backend dummy yang menghasilkan trace OTel |
| 2 | Pembangunan Observe-System + integrasi Jaeger | API gateway dengan akses data trace |
| 3 | Integrasi AI Agent (Gemini) untuk RCA | Endpoint /api/ai/diagnose berfungsi |
| 4 | Pembangunan Mobile Dashboard Flutter | Aplikasi mobile end-to-end terintegrasi |

### 3.2.2 Pemilihan Teknologi dan Framework

**Tabel 3.4 Justifikasi Pemilihan Teknologi**

| Komponen | Teknologi | Justifikasi |
|---|---|---|
| Backend API (Arthur) | Golang + Chi v5 | Performa concurrent processing, GC efisien, native OTel SDK |
| Platform Backend | Golang + Gin v1.12 | Middleware otelgin untuk auto-instrumentation HTTP |
| Distributed Tracing | Jaeger via OTel Collector | Implementasi referensi OpenTracing CNCF |
| AI/LLM | Google Gemini 2.0 Flash | Latensi < 5s, structured JSON output, biaya terjangkau |
| Mobile Dashboard | Flutter (Dart ≥3.0) | Cross-platform, hot reload, widget system ekspresif |
| Telemetry Standard | OpenTelemetry (OTel) | Vendor-neutral, semantic conventions terstandarisasi |
| Database (Auth) | JSON file storage | Cocok prototype, ringan, mudah backup |
| Authentication | API Key (custom) | Sesuai mobile-to-backend tanpa kompleksitas OAuth |




---

## 3.3 Perancangan Proses

### 3.3.1 Proses Bisnis Berjalan (AS-IS)

Pada kondisi saat ini, proses penanganan *error* pada sistem backend berbasis *microservices* dilakukan secara manual dan terfragmentasi (Brechner, 2021; Majors et al., 2022).

```
┌─────────────────────────────────────────────────────────────────────────┐
│                  ACTIVITY DIAGRAM AS-IS (PROSES MANUAL)                 │
├──────────────┬──────────────┬──────────────┬──────────────┬─────────────┤
│  Backend     │  Monitoring  │   Log Tool   │  Trace Tool  │   System    │
│   Engineer   │   Tool       │ (Kibana/Loki)│  (Jaeger)    │   Backend   │
├──────────────┼──────────────┼──────────────┼──────────────┼─────────────┤
│              │              │              │              │   ●         │
│              │              │              │              │   │ Error   │
│              │              │              │              │   ▼         │
│              │   ┌───────┐  │              │              │             │
│              │   │Detect │◄─────────────────────────────────  alert     │
│              │   └───┬───┘  │              │              │             │
│   ┌───────┐  │       │      │              │              │             │
│   │Receive│◄─────────┘      │              │              │             │
│   │ alert │  │              │              │              │             │
│   └───┬───┘  │              │              │              │             │
│       │ ⏱~5min                              │              │             │
│       ▼      │              │              │              │             │
│   ┌───────┐  │              │  ┌────────┐  │              │             │
│   │ Open  │──────────────────► │ Search │  │              │             │
│   │ logs  │  │              │  │  log   │  │              │             │
│   └───┬───┘  │              │  └────┬───┘  │              │             │
│       │ ⏱~10min (manual reading)    │              │              │     │
│       ▼      │              │              │              │             │
│   ┌───────┐  │              │              │  ┌────────┐  │             │
│   │ Open  │──────────────────────────────────►│ Search │  │             │
│   │ trace │  │              │              │  │ trace  │  │             │
│   └───┬───┘  │              │              │  └────┬───┘  │             │
│       │ ⏱~10min                            │              │             │
│       ▼      │              │              │              │             │
│   ┌─────────────────────────────┐  ⚠ BOTTLENECK              │           │
│   │ Correlate data manually     │    (data fragmented)      │            │
│   │ across multiple tools       │                            │            │
│   └─────────────┬───────────────┘                            │            │
│       │ ⏱~15min (cognitive load)                              │            │
│       ▼      │              │              │              │             │
│   ┌─────────────────────────────┐  ⚠ Bergantung pengalaman │             │
│   │ Determine root cause        │    engineer (subjektif)   │             │
│   └─────────────┬───────────────┘                            │             │
│       │      │              │              │              │             │
│       ▼      │              │              │              │             │
│   ┌───────┐                                                  │             │
│   │ Apply │                                                  │             │
│   │  fix  │                                                  │             │
│   └───┬───┘                                                  │             │
│       │                                                      │             │
│       ▼                                                      │             │
│       ●  END (~40 menit total)                               │             │
└──────────────┴──────────────┴──────────────┴──────────────┴─────────────┘
```

**Gambar 3.1** Activity Diagram Proses Bisnis Berjalan (AS-IS)

**Analisis Kelemahan AS-IS:**

1. **MTTR tinggi**: Total waktu rata-rata 40 menit per insiden karena fragmentasi data.
2. **Fragmentasi data**: Engineer harus berpindah antar minimal 3 tools terpisah.
3. **Subjektivitas diagnosis**: Hasil RCA bergantung pada pengalaman individu (Li et al., 2022).
4. **Tidak ada knowledge retention**: Setiap insiden ditangani dari nol.

### 3.3.2 Proses Bisnis yang Diusulkan (TO-BE)

Platform observability mengintegrasikan seluruh *pipeline* observability ke dalam satu sistem terpadu yang didukung kecerdasan buatan.

```
┌─────────────────────────────────────────────────────────────────────────┐
│                ACTIVITY DIAGRAM TO-BE (PROSES OTOMATIS + AI)            │
├──────────────┬──────────────┬──────────────┬──────────────┬─────────────┤
│  Backend     │   Mobile     │   Platform   │   AI Agent   │   System    │
│   Engineer   │  Dashboard   │   Backend    │   (Gemini)   │   Backend   │
├──────────────┼──────────────┼──────────────┼──────────────┼─────────────┤
│              │              │              │              │   ●         │
│              │              │              │              │   │ Error   │
│              │              │              │              │   ▼         │
│              │              │              │              │  ┌──────┐   │
│              │              │              │              │  │OTel  │   │
│              │              │              │              │  │ SDK  │   │
│              │              │              │              │  │capture│  │
│              │              │              │              │  └──┬───┘   │
│              │              │  ┌────────┐  │              │     │       │
│              │              │  │Receive │◄────OTLP gRPC───────  ┘       │
│              │              │  │via     │  │              │             │
│              │              │  │Jaeger  │  │              │             │
│              │              │  └────────┘  │              │             │
│   ┌───────┐  │              │              │              │             │
│   │ Open  │──┐              │              │              │             │
│   │ app   │  │              │              │              │             │
│   └───────┘  ▼              │              │              │             │
│              ┌─────────┐    │              │              │             │
│              │Display  │────► GET /api/    │              │             │
│              │traces   │    │ traces       │              │             │
│              │list     │◄───┤              │              │             │
│              └────┬────┘    │              │              │             │
│   ┌───────┐       │         │              │              │             │
│   │Select │       │         │              │              │             │
│   │error  │───────┘         │              │              │             │
│   │trace  │                 │              │              │             │
│   └───┬───┘                 │              │              │             │
│       │                     │              │              │             │
│   ┌───▼────┐  POST /api/ai/diagnose        │              │             │
│   │Tap     │────────────────►│              │              │             │
│   │"Diag-  │                 │  ┌────────┐  │              │             │
│   │nose AI"│                 │  │Validate│  │              │             │
│   └────────┘                 │  │API Key │  │              │             │
│                              │  └───┬────┘  │              │             │
│                              │  ┌───▼────┐  │              │             │
│                              │  │Build   │  │              │             │
│                              │  │prompt  │  │              │             │
│                              │  └───┬────┘  │              │             │
│                              │      └──────► ┌──────────┐  │             │
│                              │              │ Analyze  │  │             │
│                              │              │ via LLM  │  │             │
│                              │              │ (3-8s)   │  │             │
│                              │              └────┬─────┘  │             │
│                              │  ┌────────┐ ◄─────┘        │             │
│                              │  │Parse & │ JSON response  │             │
│                              │  │format  │                │             │
│                              │  └───┬────┘                │             │
│              ┌─────────┐     │      │                     │             │
│              │ Render  │◄────┘      │                     │             │
│              │ result  │     │                            │             │
│              └────┬────┘     │                            │             │
│   ┌──────────────▼──────┐    │                            │             │
│   │ Read diagnosis:     │    │                            │             │
│   │ - error_type        │    │                            │             │
│   │ - root_cause        │    │                            │             │
│   │ - recommendations   │    │                            │             │
│   │ - confidence_score  │    │                            │             │
│   └──────────┬──────────┘    │                            │             │
│              ▼               │                            │             │
│           ┌───────┐          │                            │             │
│           │ Apply │          │                            │             │
│           │  fix  │          │                            │             │
│           └───┬───┘          │                            │             │
│               ▼              │                            │             │
│               ●  END (~5 menit total)                     │             │
└──────────────┴──────────────┴──────────────┴──────────────┴─────────────┘
```

**Gambar 3.2** Activity Diagram Proses Bisnis yang Diusulkan (TO-BE)

**Analisis Keunggulan TO-BE:**

1. **MTTR berkurang ~87%**: Estimasi waktu turun dari ~40 menit menjadi ~5 menit.
2. **Single source of truth**: Engineer tidak berpindah antar tools.
3. **Diagnosis konsisten**: AI memberikan format analisis yang seragam.
4. **Knowledge captured**: Hasil diagnosis dapat disimpan untuk referensi (IBM Research, 2023).




---

## 3.4 Perancangan Sistem/Aplikasi

### 3.4.1 Component Diagram

Arsitektur sistem terdiri dari empat *layer* utama yang membentuk *pipeline data end-to-end*.

```
┌────────────────────────────────────────────────────────────────────────┐
│                     LAYER 1: TELEMETRY SOURCE                          │
├────────────────────────────────────────────────────────────────────────┤
│   ┌──────────────────────────────────────────────────────────────┐    │
│   │           ARTHUR API (Golang + Chi v5) — Port :8420          │    │
│   │  ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐ ┌────────────┐ │    │
│   │  │/health │ │/market │ │/trade  │ │/sim    │ │/playbooks  │ │    │
│   │  └────────┘ └────────┘ └────────┘ └────────┘ └────────────┘ │    │
│   │              ↓ OpenTelemetry SDK (auto-instrumentation)      │    │
│   └──────────────────────────────────────────────────────────────┘    │
└────────────────────────────────┬───────────────────────────────────────┘
                                 │ gRPC/OTLP
                                 ▼
┌────────────────────────────────────────────────────────────────────────┐
│                     LAYER 2: TELEMETRY PIPELINE                        │
├────────────────────────────────────────────────────────────────────────┤
│   ┌──────────────────┐                  ┌──────────────────────┐      │
│   │  OTel Collector  │ ────OTLP────────►│   Jaeger Backend     │      │
│   │   (Port :4317)   │                  │   (Port :16686)      │      │
│   │  - receive       │                  │  - trace storage     │      │
│   │  - batch         │                  │  - query API         │      │
│   │  - export        │                  │                      │      │
│   └──────────────────┘                  └──────────┬───────────┘      │
└────────────────────────────────────────────────────┼───────────────────┘
                                                     │ REST API
                                                     ▼
┌────────────────────────────────────────────────────────────────────────┐
│                     LAYER 3: PLATFORM BACKEND                          │
├────────────────────────────────────────────────────────────────────────┤
│   ┌──────────────────────────────────────────────────────────────┐    │
│   │       OBSERVE-SYSTEM (Golang + Gin v1.12) — Port :8090       │    │
│   │  ┌──────────────────┐  ┌──────────────────┐                  │    │
│   │  │  Auth Middleware │  │  CORS Middleware │                  │    │
│   │  └────────┬─────────┘  └──────────────────┘                  │    │
│   │           ▼                                                   │    │
│   │  ┌──────────────────────────────────────────────┐            │    │
│   │  │              HANDLER MODULES                  │            │    │
│   │  │  ┌──────────┐ ┌──────────┐ ┌──────────────┐  │            │    │
│   │  │  │  Traces  │ │ Metrics  │ │  AI Handler  │  │            │    │
│   │  │  │ Handler  │ │ Handler  │ │   (Gemini)   │  │            │    │
│   │  │  └──────────┘ └──────────┘ └──────┬───────┘  │            │    │
│   │  │  ┌──────────┐ ┌──────────┐         │         │            │    │
│   │  │  │ Arthur   │ │  Auth    │         │         │            │    │
│   │  │  │  Proxy   │ │ Handler  │         │         │            │    │
│   │  │  └──────────┘ └──────────┘         │         │            │    │
│   │  └────────────────────────────────────┼─────────┘            │    │
│   │                                        ▼                      │    │
│   │              ┌────────────────────────────────────┐          │    │
│   │              │       DATA STORE                    │          │    │
│   │              │   - api_keys.json                   │          │    │
│   │              │   - in-memory trace cache           │          │    │
│   │              │   - in-memory diagnosis cache       │          │    │
│   │              └────────────────────────────────────┘          │    │
│   └──────────────────────────────────────────────────────────────┘    │
└──────────────┬───────────────────────────────────────┬─────────────────┘
               │ REST + JSON                           │ HTTPS
               │ + API Key                             │
               ▼                                       ▼
┌──────────────────────────────────┐    ┌─────────────────────────────────┐
│   LAYER 4: PRESENTATION          │    │     EXTERNAL SERVICES           │
├──────────────────────────────────┤    ├─────────────────────────────────┤
│  ┌────────────────────────────┐  │    │  ┌──────────────────────────┐   │
│  │  OBSERVE-FLUTTER           │  │    │  │  Google Gemini 2.0 Flash │   │
│  │  Mobile Dashboard          │  │    │  │  - LLM API               │   │
│  │  ┌──────────────────────┐  │  │    │  │  - prompt-based RCA      │   │
│  │  │ Dashboard │ Market   │  │  │    │  │  - JSON output           │   │
│  │  │ Traces    │ AI Tab   │  │  │    │  └──────────────────────────┘   │
│  │  └──────────────────────┘  │  │    │                                 │
│  └────────────────────────────┘  │    │                                 │
└──────────────────────────────────┘    └─────────────────────────────────┘
```

**Gambar 3.3** Component Diagram Arsitektur Platform Observability

**Tabel 3.5 Deskripsi Komponen Arsitektur**

| Komponen | Teknologi | Port | Fungsi |
|---|---|---|---|
| Arthur API | Golang + Chi v5 | 8420 | Sumber data telemetri (mock backend microservices) |
| OTel Collector | OpenTelemetry Collector | 4317 | Receive, batch, dan export telemetri ke Jaeger |
| Jaeger | Jaeger All-in-One | 16686 | Trace storage dan query API |
| Observe-System | Golang + Gin v1.12 | 8090 | API gateway, AI integration, autentikasi |
| Observe-Flutter | Flutter/Dart | - | Mobile dashboard untuk monitoring & diagnosis |
| Google Gemini | Gemini 2.0 Flash | - | LLM eksternal untuk RCA |




### 3.4.2 Use Case Diagram

```
                        ╔═══════════════════════════════════════╗
                        ║     PLATFORM OBSERVABILITY SYSTEM     ║
                        ╠═══════════════════════════════════════╣
                        ║                                       ║
                        ║       ┌─────────────────────┐         ║
                ┌───────╫──────►│  UC-01              │         ║
                │       ║       │  Lihat Dashboard    │         ║
                │       ║       │  System Health      │         ║
                │       ║       └─────────────────────┘         ║
                │       ║                                       ║
                │       ║       ┌─────────────────────┐         ║
                ├───────╫──────►│  UC-02              │         ║
                │       ║       │  Lihat Daftar       │         ║
                │       ║       │  Traces             │         ║
                │       ║       └──────────┬──────────┘         ║
                │       ║                  │ <<extend>>         ║
                │       ║                  ▼                    ║
                │       ║       ┌─────────────────────┐         ║
                ├───────╫──────►│  UC-03              │         ║
                │       ║       │  Filter Traces      │         ║
                │       ║       │  (service/status)   │         ║
                │       ║       └─────────────────────┘         ║
                │       ║                                       ║
                │       ║       ┌─────────────────────┐         ║
                ├───────╫──────►│  UC-04              │         ║
                │       ║       │  Lihat Detail Span  │         ║
                │       ║       └─────────────────────┘         ║
                │       ║                                       ║
                │       ║       ┌─────────────────────┐         ║
                ├───────╫──────►│  UC-05              │         ║
                │       ║       │  Minta Diagnosis    │         ║
                ┌──┴─┐   ║       │  AI                 │         ║
                │ 👤 │   ║       └──────────┬──────────┘         ║
                ├────┤   ║                  │ <<include>>        ║
                │BE  │   ║                  ▼                    ║
                └────┘   ║       ┌─────────────────────┐         ║
                Backend  ║       │  UC-06              │         ║
                Engineer ║       │  Lihat Hasil        │         ║
                (Aktor)  ║       │  Diagnosis AI       │         ║
                │       ║       └─────────────────────┘         ║
                │       ║                                       ║
                │       ║       ┌─────────────────────┐         ║
                ├───────╫──────►│  UC-07              │         ║
                │       ║       │  Get AI System      │         ║
                │       ║       │  Summary            │         ║
                │       ║       └─────────────────────┘         ║
                │       ║                                       ║
                │       ║       ┌─────────────────────┐         ║
                └───────╫──────►│  UC-08              │         ║
                        ║       │  Lihat Market       │         ║
                        ║       │  Intelligence       │         ║
                        ║       └─────────────────────┘         ║
                        ╚═══════════════════════════════════════╝
```

**Gambar 3.4** Use Case Diagram Platform Observability

**Tabel 3.6 Deskripsi Use Case**

| ID | Nama Use Case | Aktor | Pre-condition | Post-condition |
|---|---|---|---|---|
| UC-01 | Lihat Dashboard System Health | Backend Engineer | Aplikasi terbuka | Status sistem ditampilkan |
| UC-02 | Lihat Daftar Traces | Backend Engineer | Tab Traces aktif | Daftar trace muncul |
| UC-03 | Filter Traces | Backend Engineer | UC-02 berjalan | Trace ter-filter |
| UC-04 | Lihat Detail Span | Backend Engineer | Trace dipilih | Bottom sheet detail span |
| UC-05 | Minta Diagnosis AI | Backend Engineer | Trace error dipilih | Request dikirim ke AI |
| UC-06 | Lihat Hasil Diagnosis AI | Backend Engineer | UC-05 selesai | Hasil RCA ditampilkan |
| UC-07 | Get AI System Summary | Backend Engineer | Tab AI aktif | Ringkasan sistem dari AI |
| UC-08 | Lihat Market Intelligence | Backend Engineer | Tab Market aktif | Data market ditampilkan |




### 3.4.3 Sequence Diagram

Sequence Diagram menggambarkan alur interaksi antar-komponen untuk skenario utama, yaitu *AI Diagnosis* (UC-05 dan UC-06).

```
 Engineer    Mobile App    Auth MW    Platform     AI Handler   Gemini API   Jaeger
    │            │            │           │             │            │           │
    │ tap        │            │           │             │            │           │
    │"Diag AI"   │            │           │             │            │           │
    ├───────────►│            │           │             │            │           │
    │            │            │           │             │            │           │
    │            │ POST /api/ai/diagnose  │             │            │           │
    │            │ +Bearer token          │             │            │           │
    │            ├───────────►│           │             │            │           │
    │            │            │           │             │            │           │
    │            │            │ validate  │             │            │           │
    │            │            │ API key   │             │            │           │
    │            │            ├──────────►│             │            │           │
    │            │            │   ✓valid  │             │            │           │
    │            │            │◄──────────┤             │            │           │
    │            │            │           │             │            │           │
    │            │            │           │ forward     │            │           │
    │            │            │           │ request     │            │           │
    │            │            │           ├────────────►│            │           │
    │            │            │           │             │            │           │
    │            │            │           │             │ build      │           │
    │            │            │           │             │ prompt     │           │
    │            │            │           │             ├──┐         │           │
    │            │            │           │             │◄─┘         │           │
    │            │            │           │             │            │           │
    │            │            │           │             │ POST       │           │
    │            │            │           │             │ /generate  │           │
    │            │            │           │             ├───────────►│           │
    │            │            │           │             │            │           │
    │            │            │           │             │            │ analyze   │
    │            │            │           │             │            │ trace     │
    │            │            │           │             │            ├──┐~3-8s   │
    │            │            │           │             │            │◄─┘        │
    │            │            │           │             │            │           │
    │            │            │           │             │ JSON RCA   │           │
    │            │            │           │             │ response   │           │
    │            │            │           │             │◄───────────┤           │
    │            │            │           │             │            │           │
    │            │            │           │             │ parse &    │           │
    │            │            │           │             │ validate   │           │
    │            │            │           │             ├──┐         │           │
    │            │            │           │             │◄─┘         │           │
    │            │            │           │             │            │           │
    │            │            │           │ Diagnose    │            │           │
    │            │            │           │ Response    │            │           │
    │            │            │           │◄────────────┤            │           │
    │            │            │           │             │            │           │
    │            │ HTTP 200   │           │             │            │           │
    │            │ + JSON RCA │           │             │            │           │
    │            │◄───────────┴───────────┤             │            │           │
    │            │                        │             │            │           │
    │            │ render result          │             │            │           │
    │            ├──┐                     │             │            │           │
    │            │◄─┘                     │             │            │           │
    │            │                        │             │            │           │
    │ display    │                        │             │            │           │
    │ diagnosis  │                        │             │            │           │
    │◄───────────┤                        │             │            │           │
    │            │                        │             │            │           │
```

**Gambar 3.5** Sequence Diagram untuk Use Case Diagnosis AI (UC-05, UC-06)

### 3.4.4 Entity Relationship Diagram (ERD)

```
┌─────────────────────────────────┐
│           api_keys              │
├─────────────────────────────────┤
│ PK  key            (string)     │
│     name           (string)     │
│     description    (string)     │
│     arthur_url     (string)     │
│     jaeger_url     (string)     │
│     created_at     (timestamp)  │
│     last_used_at   (timestamp)  │
│     revoked        (boolean)    │
└────────────────┬────────────────┘
                 │ 1
                 │
                 │ N (uses)
                 ▼
┌─────────────────────────────────┐         ┌────────────────────────────┐
│           services              │ 1     N │      trace_cache            │
├─────────────────────────────────┤◄────────┤────────────────────────────┤
│ PK  service_id     (uuid)       │         │ PK  trace_id   (string)     │
│     service_name   (string)     │         │ FK  service_id (uuid)       │
│     environment    (string)     │         │     operation  (string)     │
│     last_seen      (timestamp)  │         │     status     (enum)       │
│     instrumented   (boolean)    │         │     duration_ms(float)      │
└─────────────────────────────────┘         │     timestamp  (timestamp)  │
                                             │     spans      (jsonb)     │
                                             │     log_message(text)      │
                                             └────────────┬───────────────┘
                                                          │ 1
                                                          │
                                                          │ 0..1
                                                          ▼
                                             ┌─────────────────────────────┐
                                             │      ai_diagnoses           │
                                             ├─────────────────────────────┤
                                             │ PK  diagnosis_id (uuid)     │
                                             │ FK  trace_id     (string)   │
                                             │     error_type   (string)   │
                                             │     root_cause   (text)     │
                                             │     affected_services       │
                                             │                  (jsonb)    │
                                             │     recommendation          │
                                             │                  (jsonb)    │
                                             │     confidence_score (float)│
                                             │     analyzed_at  (timestamp)│
                                             │     llm_model    (string)   │
                                             └─────────────────────────────┘
```

**Gambar 3.6** Entity Relationship Diagram Platform Observability

**Tabel 3.7 Penjelasan Entitas dan Relasi**

| Entitas | Deskripsi | Relasi |
|---|---|---|
| `api_keys` | Menyimpan API key untuk autentikasi mobile client | 1 : N dengan `services` |
| `services` | Registry service backend yang dipantau | 1 : N dengan `trace_cache` |
| `trace_cache` | Cache data trace dari Jaeger | 1 : 0..1 dengan `ai_diagnoses` |
| `ai_diagnoses` | Hasil RCA dari Gemini AI | - |

**Catatan**: Pada *prototype* saat ini, `api_keys` disimpan dalam JSON file. Untuk produksi, direkomendasikan PostgreSQL dengan partisi *time-series* untuk `trace_cache` (Kleppmann, 2017).




### 3.4.5 Class Diagram

```
┌───────────────────────────┐         ┌──────────────────────────────┐
│       << package >>       │         │       << package >>          │
│           auth            │         │            ai                │
├───────────────────────────┤         ├──────────────────────────────┤
│  + Store                  │         │  + Handler                   │
│    - keys: map            │         │    - gemini: GeminiConfig    │
│    - filePath: string     │         │    - arthur: ArthurConfig    │
│    + Create()             │         │    - client: *http.Client    │
│    + Get()                │         │    + Diagnose(c)             │
│    + Touch()              │         │    + SystemSummary(c)        │
│    + Revoke()             │         │    + SectionInsight(c)       │
│    + Delete()             │         │    - callGemini(prompt)      │
│  + Middleware()           │         │    - buildDiagnosePrompt()   │
│  + APIKey                 │         │  + DiagnoseRequest           │
│    - key: string          │         │    - TraceID: string         │
│    - name: string         │         │    - ServiceName: string     │
│    - createdAt: time      │         │    - Spans: []SpanInfo       │
│    - revoked: bool        │         │  + DiagnoseResponse          │
└─────────────┬─────────────┘         │    - ErrorType: string       │
              │ uses                   │    - RootCause: string       │
              │                       │    - Recommendation: []string│
              ▼                       │    - ConfidenceScore: float  │
┌───────────────────────────┐         └──────────────────────────────┘
│       << package >>       │                       ▲
│         traces            │                       │ uses
├───────────────────────────┤         ┌─────────────┴────────────────┐
│  + Handler                │         │       << package >>           │
│    - jaegerBase: string   │         │           config              │
│    - client: *http.Client │         ├───────────────────────────────┤
│    + ListTraces(c)        │◄────────┤  + GeminiConfig               │
│    + GetTrace(c)          │  uses   │    - APIKey: string           │
│  + SimpleTrace            │         │    - APIURL: string           │
│    - TraceID: string      │         │    - Temperature: float       │
│    - Service: string      │         │  + ArthurConfig               │
│    - Status: string       │         │    - BaseURL: string          │
│    - DurationMs: float    │         │  + JaegerConfig               │
│    - Spans: []SimpleSpan  │         │    - BaseURL: string          │
└───────────────────────────┘         └───────────────────────────────┘
```

**Gambar 3.7** Class Diagram Platform Backend (Observe-System)

### 3.4.6 Wireframe Mobile Dashboard

**Halaman 1: Dashboard (Home)**

```
┌─────────────────────────────────┐
│  ObserveAI            🔄    👤  │
├─────────────────────────────────┤
│  ● Arthur API    🟢 Healthy     │
│  ┌─────────┬─────────┬────────┐ │
│  │  ERROR  │  OTel   │   AI   │ │
│  │   12    │ ACTIVE  │ READY  │ │
│  └─────────┴─────────┴────────┘ │
├─────────────────────────────────┤
│  🤖 AI SYSTEM SUMMARY           │
│  ┌─────────────────────────┐   │
│  │ Status: HEALTHY          │   │
│  │ "All services running    │   │
│  │  within SLO. RSI..."     │   │
│  └─────────────────────────┘   │
├─────────────────────────────────┤
│  MARKET SNAPSHOT                │
│  ┌──────────────┬────────────┐ │
│  │ BTC PRICE    │   RSI      │ │
│  │  $67,234     │   58.2     │ │
│  └──────────────┴────────────┘ │
├─────────────────────────────────┤
│  RECENT TRACES                  │
│  🟢 GET /market/snapshot 142ms  │
│  🔴 GET /market/alerts  5300ms  │
│  🟢 GET /playbooks       23ms   │
├──────┬──────┬──────┬───────────┤
│ Home │Market│Traces│  AI       │
└──────┴──────┴──────┴───────────┘
```

**Gambar 3.8** Wireframe Halaman Dashboard

**Halaman 2: Traces**

```
┌─────────────────────────────────┐
│  OTel Traces  [arthur-api ▼] 🔄 │
├─────────────────────────────────┤
│  [All (30)] [Error (5)] [OK 25] │
├─────────────────────────────────┤
│  ┌───────────────────────────┐  │
│  │ 🔴 GET /market/alerts      │  │
│  │ arthur-api  ·  5300ms      │  │
│  │ 2 spans                    │  │
│  │ 2024-05-25T12:34:01Z       │  │
│  │              [🤖 Diagnose] │  │
│  └───────────────────────────┘  │
│  ┌───────────────────────────┐  │
│  │ 🟢 GET /market/snapshot    │  │
│  │ arthur-api  ·  142ms       │  │
│  │ 3 spans                    │  │
│  │ 2024-05-25T12:33:55Z       │  │
│  └───────────────────────────┘  │
├──────┬──────┬──────┬───────────┤
│ Home │Market│Traces│   AI      │
└──────┴──────┴──────┴───────────┘
```

**Gambar 3.9** Wireframe Halaman Traces

**Halaman 3: AI Diagnosis Detail**

```
┌─────────────────────────────────┐
│  ← AI Diagnosis Result      🔄  │
├─────────────────────────────────┤
│  TRACE                          │
│  abc123def456                   │
│  GET /market/alerts             │
├─────────────────────────────────┤
│  🤖 ERROR TYPE                  │
│  ┌───────────────────────────┐  │
│  │ External Service Timeout  │  │
│  └───────────────────────────┘  │
│                                 │
│  🔍 ROOT CAUSE                  │
│  ┌───────────────────────────┐  │
│  │ Upstream Bybit API        │  │
│  │ connection timed out      │  │
│  │ after 5000ms during       │  │
│  │ price fetch operation...  │  │
│  └───────────────────────────┘  │
│                                 │
│  AFFECTED SERVICES              │
│  [arthur-api] [bybit-fetcher]   │
│                                 │
│  RECOMMENDATIONS                │
│  ┌───────────────────────────┐  │
│  │ 1. Implement circuit      │  │
│  │    breaker pattern        │  │
│  │ 2. Add retry with         │  │
│  │    exponential backoff    │  │
│  │ 3. Configure shorter      │  │
│  │    timeout with fallback  │  │
│  └───────────────────────────┘  │
│                                 │
│  CONFIDENCE SCORE               │
│  ████████░░  87%                │
│                                 │
│  Analyzed at 2024-05-25T12:35Z  │
└─────────────────────────────────┘
```

**Gambar 3.10** Wireframe Halaman AI Diagnosis Detail

**Halaman 4: AI Tab**

```
┌─────────────────────────────────┐
│  AI Diagnosis                   │
├─────────────────────────────────┤
│  ┌───────────────────────────┐  │
│  │  🤖 AI — System           │  │
│  │     Intelligence          │  │
│  │                           │  │
│  │  AI analyzes Arthur API   │  │
│  │  traces and logs to       │  │
│  │  perform Root Cause       │  │
│  │  Analysis of backend      │  │
│  │  errors.                  │  │
│  │                           │  │
│  │  [🔄 Get System Summary] │  │
│  └───────────────────────────┘  │
├─────────────────────────────────┤
│  ● HEALTHY                      │
│  "System running normally..."   │
│                                 │
│  KEY SIGNALS                    │
│  • Latency p95: 145ms           │
│  • Error rate: 0.85%            │
│  • Active services: 5           │
│                                 │
│  RECOMMENDATION                 │
│  Continue monitoring...         │
├─────────────────────────────────┤
│  HOW TO DIAGNOSE A TRACE        │
│  1. Go to Traces tab            │
│  2. Tap "Diagnose AI"           │
│  3. AI analyzes spans...        │
├──────┬──────┬──────┬───────────┤
│ Home │Market│Traces│  AI 👈    │
└──────┴──────┴──────┴───────────┘
```

**Gambar 3.11** Wireframe Halaman AI Tab




### 3.4.7 Algoritma dan Metode Kecerdasan Buatan

Komponen AI menggunakan pendekatan *prompt engineering* terhadap Google Gemini 2.0 Flash melalui REST API. Pemilihan pendekatan ini sesuai batasan penelitian yang menetapkan penggunaan layanan LLM tanpa proses *training* model baru (Chen et al., 2022).

**Arsitektur AI Agent terdiri dari tiga lapisan:**

1. **Data Preprocessing Layer** — Menormalisasi dan memformat data *log* dan *trace*.
2. **Prompt Engineering Layer** — Membangun *prompt* terstruktur berdasarkan *template*.
3. **LLM Invocation Layer** — Mengirim *prompt* ke Gemini API dan mem-*parsing* respons.

**Struktur Prompt untuk RCA:**

```
=== SYSTEM PROMPT ===
You are a senior backend engineer and SRE expert specializing in
distributed systems observability and root cause analysis.
Analyze the provided log and trace data, then return ONLY a valid
JSON object without any preamble.

=== USER PROMPT (dibangun secara dinamis) ===
Analyze this backend error:

SERVICE:    {service_name}
OPERATION:  {operation}
STATUS:     {status}
DURATION:   {duration_ms} ms
TRACE ID:   {trace_id}

LOG MESSAGE:
{log_message}

TRACE SPANS:
{span_id} [{service}] {operation} → {status} ({duration}ms)
{span_id} [{service}] {operation} → {status} ({duration}ms)

EXTRA CONTEXT:
{extra_ctx}

Return ONLY a valid JSON object with this exact structure:
{
  "error_type":         "string",
  "root_cause":         "string",
  "affected_services":  ["string"],
  "recommendation":     ["string"],
  "confidence_score":   float (0.0–1.0)
}
```

**Gambar 3.12** Struktur Prompt Template AI Agent

**Algoritma Preprocessing Data Telemetri:**

1. **Normalisasi Format**: Konversi *timestamp* ke ISO 8601, standardisasi nama *service*.
2. **Penyaringan Span**: Hanya *span* dengan status *error* atau *duration* > threshold.
3. **Konstruksi Hierarki Trace**: *Span* diurutkan berdasarkan *parent_span_id*.
4. **Pembatasan Token**: Total *prompt* dibatasi ≤ 4000 token.

**Konfigurasi LLM:**

| Parameter | Nilai | Justifikasi |
|---|---|---|
| Model | gemini-2.0-flash | Latensi rendah, structured output |
| Temperature | 0.1 | Konsistensi output (deterministik) |
| MaxOutputTokens | 1500 | Cukup untuk RCA terstruktur |
| Timeout | 30 detik | Sesuai NFR-01 |

---

## 3.5 Perancangan Pengujian

### 3.5.1 Strategi Pengujian

Pengujian dilakukan menggunakan metode *User Acceptance Testing* (UAT) dengan pendekatan *scenario-based testing*. Setiap skenario merepresentasikan jenis *error* yang umum terjadi pada sistem backend berbasis *microservices* (Brechner, 2021; Li et al., 2023). Setiap skenario dijalankan **3 kali** untuk memvalidasi konsistensi *output* AI.

### 3.5.2 Lingkungan Pengujian

| Komponen | Spesifikasi |
|---|---|
| Hardware | MacBook Apple Silicon, RAM 16 GB |
| OS | macOS / Linux (Docker) |
| Arthur API | localhost:8420 |
| OTel Collector | localhost:4317 (gRPC) |
| Jaeger | localhost:16686 |
| Observe-System | localhost:8090 |
| Mobile App | Android Emulator API 33 |
| Gemini API | Google AI Studio (cloud) |

### 3.5.3 Skenario User Acceptance Testing

**Tabel 3.8 Rancangan User Acceptance Testing (UAT)**

| No | Skenario Error | Input (Simulasi) | Expected Output AI | Hasil Aktual | Status |
|---|---|---|---|---|---|
| 1 | External Service Timeout | Request /market/snapshot memicu timeout pada koneksi Bybit (>5000ms) | AI mengidentifikasi timeout, menyebut service pemanggil, merekomendasikan circuit breaker | [ ] | [ ] |
| 2 | HTTP 500 Internal Server Error | /market/alerts mengalami nil pointer pada data processing | AI mendeteksi internal error, merekomendasikan null-check & defensive programming | [ ] | [ ] |
| 3 | High Latency / Slow Response | /market/snapshot membutuhkan >3000ms karena agregasi multiple sources | AI mengidentifikasi bottleneck, menyebut span terlama, rekomendasi caching/parallel | [ ] | [ ] |
| 4 | Cascading Service Failure | Error koneksi Bybit menyebabkan kegagalan computation indicators | AI melakukan RCA lintas-span, mengidentifikasi span pertama yang gagal | [ ] | [ ] |
| 5 | Data Source Unavailability | Service news/sentiment gagal karena API pihak ketiga tidak merespons | AI mendeteksi external dependency failure, rekomendasi fallback mechanism | [ ] | [ ] |
| 6 | Rate Limiting / Throttling | Multiple rapid requests memicu throttling upstream | AI mengidentifikasi pola throttling, rekomendasi request batching | [ ] | [ ] |
| 7 | Configuration Error | Endpoint error karena missing environment variable | AI mengidentifikasi configuration failure, rekomendasi validasi startup | [ ] | [ ] |

### 3.5.4 Kriteria Keberhasilan

AI Agent dinyatakan **BERHASIL (PASS)** untuk satu skenario apabila memenuhi **ketiga kriteria** berikut:

1. ✅ Mampu mengidentifikasi jenis *error* dengan benar (`error_type` sesuai kategori sebenarnya).
2. ✅ Memberikan *root cause* yang relevan dan spesifik (bukan analisis generik).
3. ✅ Memberikan rekomendasi yang *applicable* (sesuai *best practice* sistem terdistribusi).

**Target keseluruhan**: minimal **5 dari 7 skenario PASS (≥ 71%)** sesuai standar evaluasi AIOps pada tahap purwarupa (IBM Research, 2023).

**Confidence Score** diharapkan berada pada rentang **0.6 – 1.0** untuk skenario yang berhasil diidentifikasi. *Score* di bawah 0.6 menandakan AI tidak yakin dengan diagnosis yang diberikan.

Kolom **"Hasil Aktual"** dan **"Status"** pada Tabel 3.8 akan diisi pada Bab 4 setelah pengujian dilaksanakan dengan nilai `PASS` atau `FAIL`.


### 3.4.8 Deployment Diagram

*Deployment Diagram* menggambarkan distribusi fisik komponen pada infrastruktur saat sistem dijalankan.

```
┌──────────────────────────────────────────────────────────────────────┐
│                    DEPLOYMENT ARCHITECTURE                            │
└──────────────────────────────────────────────────────────────────────┘

┌────────────────────────────┐       ┌─────────────────────────────────┐
│   <<device>>               │       │   <<device>>                    │
│   Engineer's Mobile Phone  │       │   Local Development Server      │
│   (Android API ≥33)        │       │   (Linux/macOS, Docker)         │
├────────────────────────────┤       ├─────────────────────────────────┤
│  ┌──────────────────────┐  │       │  ┌────────────────────────────┐ │
│  │ <<artifact>>         │  │       │  │ <<container>>              │ │
│  │  observe_flutter.apk │  │       │  │  arthur-api:latest         │ │
│  │  (Flutter app)       │  │       │  │  Port: 8420                │ │
│  │                      │  │       │  └────────────────────────────┘ │
│  │  - Dashboard         │  │       │  ┌────────────────────────────┐ │
│  │  - Traces            │◄─┼───────┼──┤ <<container>>              │ │
│  │  - AI Diagnosis      │  │ HTTPS │  │  observe-system:latest     │ │
│  │  - Market            │  │ JSON  │  │  Port: 8090                │ │
│  └──────────────────────┘  │       │  │  - Auth Middleware          │ │
└────────────────────────────┘       │  │  - Handler modules          │ │
                                      │  │  - Volume: api_keys.json    │ │
                                      │  └────────────┬───────────────┘ │
                                      │               │ gRPC :4317      │
                                      │               ▼                  │
                                      │  ┌────────────────────────────┐ │
                                      │  │ <<container>>              │ │
                                      │  │  otel-collector:latest     │ │
                                      │  │  Port: 4317 (gRPC OTLP)    │ │
                                      │  └────────────┬───────────────┘ │
                                      │               │ OTLP            │
                                      │               ▼                  │
                                      │  ┌────────────────────────────┐ │
                                      │  │ <<container>>              │ │
                                      │  │  jaeger-all-in-one:latest  │ │
                                      │  │  Port: 16686 (Query UI)    │ │
                                      │  └────────────────────────────┘ │
                                      └────────────────┬─────────────────┘
                                                       │ HTTPS
                                                       ▼
                                      ┌─────────────────────────────────┐
                                      │   <<cloud>>                     │
                                      │   Google Cloud Platform         │
                                      ├─────────────────────────────────┤
                                      │  Gemini 2.0 Flash API           │
                                      │  generativelanguage.            │
                                      │  googleapis.com                 │
                                      └─────────────────────────────────┘
```

**Gambar 3.13** Deployment Diagram Platform Observability

**Tabel 3.9 Spesifikasi Deployment**

| Node | Tipe | Konten | Persyaratan |
|---|---|---|---|
| Mobile Phone | Device | observe_flutter.apk | Android ≥ API 33, RAM ≥ 2GB |
| Development Server | Device | 4 Docker container | OS Linux/macOS, RAM ≥ 8GB |
| Google Cloud | Cloud Service | Gemini 2.0 Flash API | Internet connection, API key valid |


### 3.4.9 Spesifikasi API Endpoint

API yang disediakan oleh Observe-System mengikuti prinsip RESTful dengan format JSON.

**Tabel 3.10 Spesifikasi API Endpoint Observe-System**

| Method | Endpoint | Auth | Deskripsi |
|---|---|---|---|
| GET | /health | Public | Health check |
| POST | /admin/keys | Admin Secret | Buat API key baru |
| GET | /admin/keys | Admin Secret | List API key (masked) |
| POST | /admin/keys/revoke | Admin Secret | Revoke API key |
| DELETE | /admin/keys | Admin Secret | Hapus API key |
| GET | /api/auth/verify | API Key | Validasi API key |
| GET | /api/arthur/health | API Key | Proxy Arthur health |
| GET | /api/arthur/snapshot | API Key | Market snapshot dari Arthur |
| GET | /api/arthur/alerts | API Key | Market alerts |
| GET | /api/arthur/performance | API Key | Trade performance |
| GET | /api/traces | API Key | List traces |
| GET | /api/traces/:traceID | API Key | Detail satu trace |
| GET | /api/metrics/services | API Key | RED metrics per service |
| GET | /api/metrics/service/:name | API Key | Timeseries per service |
| **POST** | **/api/ai/diagnose** | **API Key** | **AI diagnosis trace (UTAMA)** |
| GET | /api/ai/summary | API Key | AI system summary |
| POST | /api/ai/insight | API Key | Per-section AI insight |

**Detail Schema Request/Response untuk Endpoint Diagnose:**

```json
// REQUEST: POST /api/ai/diagnose
{
  "trace_id":     "abc123def456",
  "service_name": "arthur-api",
  "operation":    "GET /market/alerts",
  "status":       "error",
  "duration_ms":  5300.5,
  "spans": [
    {
      "span_id":     "s004",
      "operation":   "GET /market/alerts",
      "duration_ms": 5300,
      "status":      "error"
    },
    {
      "span_id":     "s005",
      "operation":   "hyperliquid.FetchFunding",
      "duration_ms": 5290,
      "status":      "error"
    }
  ],
  "log_message": "Error trace from Arthur API",
  "extra_ctx": {}
}

// RESPONSE: HTTP 200 OK
{
  "error_type":        "External Service Timeout",
  "root_cause":        "Upstream Hyperliquid API connection timed out after 5290ms during funding rate fetch operation",
  "affected_services": ["arthur-api", "hyperliquid-fetcher"],
  "recommendation": [
    "Implement circuit breaker pattern with 3000ms timeout",
    "Add retry with exponential backoff (max 3 attempts)",
    "Configure fallback to cached data when upstream is slow"
  ],
  "confidence_score": 0.87,
  "analyzed_at":      "2024-05-25T12:35:18Z"
}
```

**Gambar 3.14** Schema Request/Response Endpoint AI Diagnose


### 3.4.10 Skema Basis Data (DDL)

Untuk migrasi ke production-grade database (PostgreSQL), skema DDL dirancang sebagai berikut. Pada implementasi prototype, skema disederhanakan menjadi in-memory dan JSON file.

```sql
-- ===================================================================
-- TABEL 1: api_keys
-- ===================================================================
CREATE TABLE api_keys (
    key             VARCHAR(64)  PRIMARY KEY,
    name            VARCHAR(100) NOT NULL,
    description     TEXT,
    arthur_url      VARCHAR(255) NOT NULL,
    jaeger_url      VARCHAR(255) NOT NULL,
    created_at      TIMESTAMP    NOT NULL DEFAULT NOW(),
    last_used_at    TIMESTAMP,
    revoked         BOOLEAN      NOT NULL DEFAULT FALSE
);
CREATE INDEX idx_api_keys_revoked ON api_keys(revoked);

-- ===================================================================
-- TABEL 2: services
-- ===================================================================
CREATE TABLE services (
    service_id      UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    service_name    VARCHAR(100) NOT NULL UNIQUE,
    environment     VARCHAR(20)  NOT NULL DEFAULT 'production',
    last_seen       TIMESTAMP    NOT NULL DEFAULT NOW(),
    instrumented    BOOLEAN      NOT NULL DEFAULT TRUE
);
CREATE INDEX idx_services_name ON services(service_name);

-- ===================================================================
-- TABEL 3: trace_cache (di-partisi per hari pada production)
-- ===================================================================
CREATE TABLE trace_cache (
    trace_id        VARCHAR(64)  PRIMARY KEY,
    service_id      UUID         NOT NULL REFERENCES services(service_id),
    operation       VARCHAR(255) NOT NULL,
    status          VARCHAR(10)  NOT NULL CHECK (status IN ('ok','error','warn')),
    duration_ms     FLOAT        NOT NULL,
    timestamp       TIMESTAMP    NOT NULL,
    spans           JSONB        NOT NULL,
    log_message     TEXT,
    cached_at       TIMESTAMP    NOT NULL DEFAULT NOW()
) PARTITION BY RANGE (timestamp);

CREATE INDEX idx_trace_service     ON trace_cache(service_id);
CREATE INDEX idx_trace_status      ON trace_cache(status);
CREATE INDEX idx_trace_timestamp   ON trace_cache(timestamp DESC);

-- ===================================================================
-- TABEL 4: ai_diagnoses
-- ===================================================================
CREATE TABLE ai_diagnoses (
    diagnosis_id        UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    trace_id            VARCHAR(64)  NOT NULL REFERENCES trace_cache(trace_id),
    error_type          VARCHAR(100) NOT NULL,
    root_cause          TEXT         NOT NULL,
    affected_services   JSONB        NOT NULL,
    recommendation      JSONB        NOT NULL,
    confidence_score    FLOAT        NOT NULL CHECK (confidence_score BETWEEN 0 AND 1),
    analyzed_at         TIMESTAMP    NOT NULL DEFAULT NOW(),
    llm_model           VARCHAR(50)  NOT NULL DEFAULT 'gemini-2.0-flash',
    UNIQUE(trace_id, llm_model)
);
CREATE INDEX idx_diag_trace      ON ai_diagnoses(trace_id);
CREATE INDEX idx_diag_error_type ON ai_diagnoses(error_type);
CREATE INDEX idx_diag_analyzed   ON ai_diagnoses(analyzed_at DESC);
```

**Gambar 3.15** Skema DDL PostgreSQL untuk Production Deployment


### 3.4.11 State Diagram Trace Lifecycle

State Diagram menggambarkan transisi state dari sebuah trace sejak masuk ke sistem hingga selesai didiagnosis oleh AI.

```
                        ┌─────────────────┐
                        │   ● START       │
                        │   (trace masuk) │
                        └────────┬────────┘
                                 │
                                 ▼
                        ┌─────────────────┐
                        │  COLLECTED      │
                        │  (di Jaeger)    │
                        └────────┬────────┘
                                 │ user opens app
                                 ▼
                        ┌─────────────────┐
                        │  LISTED         │
                        │  (in mobile UI) │
                        └────────┬────────┘
                                 │
                            tap on trace
                                 │
                                 ▼
                        ┌─────────────────┐
                        │  VIEWED         │
                        │  (detail spans) │
                        └────────┬────────┘
                                 │ if status==error
                                 │ AND tap "Diagnose AI"
                                 ▼
                        ┌─────────────────┐
                        │  REQUESTED      │
                        │  (sent to API)  │
                        └────────┬────────┘
                                 │
                            ┌────┴────┐
                            ▼         ▼
                ┌─────────────┐  ┌──────────────┐
                │ AUTH_FAILED │  │ AUTHENTICATED│
                │  (401)      │  │              │
                └─────┬───────┘  └──────┬───────┘
                      │                  │
                      │                  ▼
                      │          ┌──────────────┐
                      │          │  ANALYZING   │
                      │          │  (Gemini in  │
                      │          │   progress)  │
                      │          └──────┬───────┘
                      │                  │
                      │              ┌───┴───┐
                      │              ▼       ▼
                      │      ┌────────┐ ┌───────────┐
                      │      │TIMEOUT │ │ DIAGNOSED │
                      │      │ (>30s) │ │ (success) │
                      │      └───┬────┘ └─────┬─────┘
                      │          │            │
                      ▼          ▼            ▼
              ┌────────────────────────────────────┐
              │            DISPLAYED               │
              │  (engineer reads result on UI)     │
              └─────────────────┬──────────────────┘
                                │
                                ▼
                        ┌─────────────────┐
                        │   ● END         │
                        │  (engineer      │
                        │   applies fix)  │
                        └─────────────────┘
```

**Gambar 3.16** State Diagram Trace Lifecycle dari Collection hingga Diagnosis


### 3.5.5 Detail Test Case per Skenario UAT

Setiap skenario UAT pada Tabel 3.8 dijabarkan menjadi test case detail dengan test ID, precondition, test step, dan expected result.

**Tabel 3.11 Detail Test Case TC-01 (External Service Timeout)**

| Field | Detail |
|---|---|
| Test ID | TC-01 |
| Skenario | External Service Timeout |
| Precondition | Arthur API running, OTel Collector aktif, Jaeger reachable, Mobile app login dengan API key valid |
| Test Step | 1. Hentikan layanan Bybit API (atau set firewall block)<br>2. Trigger request GET /market/snapshot pada Arthur API<br>3. Tunggu hingga trace muncul di Jaeger (max 1 menit)<br>4. Buka mobile app, tab Traces<br>5. Pilih trace dengan status error<br>6. Tap tombol "Diagnose AI"<br>7. Tunggu hasil diagnosis muncul |
| Expected Result | a. AI mengembalikan error_type mengandung kata "timeout" atau "External Service"<br>b. root_cause menyebutkan "Bybit" atau "upstream service"<br>c. recommendation mengandung minimal 1 dari: "circuit breaker", "retry", "timeout"<br>d. confidence_score >= 0.6<br>e. Response time <= 30 detik |
| Pass Criteria | a, b, c, d, e semua terpenuhi |

**Tabel 3.12 Detail Test Case TC-02 (HTTP 500 Internal Server Error)**

| Field | Detail |
|---|---|
| Test ID | TC-02 |
| Skenario | Nil Pointer / HTTP 500 |
| Precondition | Sama seperti TC-01 |
| Test Step | 1. Modifikasi handler /market/alerts untuk memicu nil pointer<br>2. Trigger request GET /market/alerts<br>3. Verifikasi trace berstatus error di Jaeger<br>4. Mobile app, tab Traces, pilih trace tersebut<br>5. Tap "Diagnose AI" |
| Expected Result | a. error_type mengandung "Nil Pointer", "Null Reference", atau "Internal Server Error"<br>b. root_cause mengidentifikasi operation yang gagal<br>c. recommendation mengandung "null check" atau "defensive programming"<br>d. confidence_score >= 0.6 |
| Pass Criteria | a, b, c, d semua terpenuhi |

**Tabel 3.13 Detail Test Case TC-03 (High Latency)**

| Field | Detail |
|---|---|
| Test ID | TC-03 |
| Skenario | High Latency / Slow Response |
| Precondition | Sama seperti TC-01 |
| Test Step | 1. Inject time.Sleep(3 second) di handler /market/snapshot<br>2. Pastikan minimal 4 span sequential ada dalam trace<br>3. Trigger request<br>4. Mobile app, Diagnose AI |
| Expected Result | a. error_type mengandung "Latency", "Performance", atau "Slow Response"<br>b. root_cause mengidentifikasi span dengan durasi tertinggi<br>c. recommendation mengandung "caching", "parallel", atau "optimize"<br>d. confidence_score >= 0.6 |
| Pass Criteria | a, b, c, d semua terpenuhi |

**Tabel 3.14 Detail Test Case TC-04 (Cascading Failure)**

| Field | Detail |
|---|---|
| Test ID | TC-04 |
| Skenario | Cascading Service Failure |
| Precondition | Sama seperti TC-01 |
| Test Step | 1. Putuskan koneksi Bybit (parent dependency)<br>2. Trigger /market/snapshot (yang juga memanggil indicator computation)<br>3. Verifikasi minimal 3 span error dalam trace<br>4. Diagnose AI |
| Expected Result | a. error_type mengandung "Cascading" atau "Chain Failure"<br>b. root_cause mengidentifikasi span pertama (root) sebagai sumber<br>c. affected_services berisi minimal 2 service<br>d. recommendation mengandung "bulkhead", "isolation", atau "graceful degradation" |
| Pass Criteria | a, b, c, d semua terpenuhi |

**Tabel 3.15 Detail Test Case TC-05 (Data Source Unavailability)**

| Field | Detail |
|---|---|
| Test ID | TC-05 |
| Skenario | Data Source Unavailability |
| Precondition | Sama seperti TC-01 |
| Test Step | 1. Set host CryptoPanic API ke unreachable<br>2. Trigger /market/news<br>3. Verifikasi span error pada news fetcher<br>4. Diagnose AI |
| Expected Result | a. error_type mengandung "Dependency", "Unavailable", atau "Connection"<br>b. root_cause menyebutkan API pihak ketiga<br>c. recommendation mengandung "fallback", "alternative source", atau "cache" |
| Pass Criteria | a, b, c semua terpenuhi |

**Tabel 3.16 Detail Test Case TC-06 (Rate Limiting)**

| Field | Detail |
|---|---|
| Test ID | TC-06 |
| Skenario | Rate Limiting / Throttling |
| Precondition | Sama seperti TC-01 |
| Test Step | 1. Trigger 50 rapid request ke /market/snapshot dalam 1 menit<br>2. Verifikasi pola latency bertahap (87ms, 245ms, 890ms, 2100ms)<br>3. Diagnose AI pada trace dengan latency tinggi |
| Expected Result | a. error_type mengandung "Rate Limit", "Throttling", atau "Quota"<br>b. root_cause mengidentifikasi pola peningkatan latency<br>c. recommendation mengandung "batching", "rate limiter", atau "caching" |
| Pass Criteria | a, b, c semua terpenuhi |

**Tabel 3.17 Detail Test Case TC-07 (Configuration Error)**

| Field | Detail |
|---|---|
| Test ID | TC-07 |
| Skenario | Configuration Error |
| Precondition | Sama seperti TC-01 |
| Test Step | 1. Hapus environment variable GEMINI_API_KEY dari Observe-System<br>2. Restart Observe-System<br>3. Mobile app, Diagnose AI<br>4. Verifikasi response berisi error config |
| Expected Result | a. error_type mengandung "Configuration" atau "Environment"<br>b. root_cause menyebutkan variable yang missing<br>c. recommendation mengandung "validate at startup" atau "fail-fast" |
| Pass Criteria | a, b, c semua terpenuhi |

---

## 3.6 Analisis Risiko dan Mitigasi

Sebagai bagian dari perencanaan, berikut analisis risiko teknis yang dapat mempengaruhi implementasi serta strategi mitigasinya.

**Tabel 3.18 Risiko Teknis dan Strategi Mitigasi**

| ID | Kategori Risiko | Deskripsi | Probabilitas | Dampak | Strategi Mitigasi |
|---|---|---|---|---|---|
| R-01 | External Dependency | Gemini API down atau breaking change | Rendah | Tinggi | Graceful fallback dengan pesan informatif; tetap tampilkan trace data |
| R-02 | Performance | Response Gemini melebihi 30 detik | Sedang | Sedang | Timeout 30s di HTTP client; UI menampilkan retry button |
| R-03 | Cost | Penggunaan Gemini API melebihi quota free tier | Sedang | Sedang | Cache hasil diagnosis berdasarkan trace_id; rate limiting |
| R-04 | Security | API key bocor pada mobile binary | Sedang | Tinggi | API key di-scope per pengguna; mekanisme revoke; future: OAuth2 |
| R-05 | Data Quality | Trace dari OTel kurang detail (missing tags) | Tinggi | Sedang | Validasi span attributes saat preprocessing; minimum required fields |
| R-06 | Reliability | Jaeger crash, trace data hilang | Rendah | Tinggi | Mock data fallback; trace cache di Observe-System; persistent storage |
| R-07 | LLM Hallucination | AI memberikan diagnosis tidak akurat | Sedang | Sedang | Confidence score threshold 0.6; engineer tetap memvalidasi sebelum apply fix |
| R-08 | Compatibility | Flutter app tidak kompatibel dengan Android lama | Rendah | Rendah | Minimum API 33; testing pada multiple device |
