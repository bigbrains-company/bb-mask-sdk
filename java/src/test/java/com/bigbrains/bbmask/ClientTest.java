package com.bigbrains.bbmask;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpServer;
import org.junit.jupiter.api.Test;

import java.io.IOException;
import java.net.InetSocketAddress;
import java.net.ServerSocket;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.time.Duration;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.atomic.AtomicInteger;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class ClientTest {
    private static final ObjectMapper JSON = new ObjectMapper();
    private static final TypeReference<Map<String, Object>> MAP = new TypeReference<>() {
    };

    @Test
    void anonymizeSendsWireBody() throws Exception {
        List<Map<String, Object>> seen = new ArrayList<>();
        try (Probe server = new Probe(exchange -> {
            seen.add(Map.of(
                    "method", exchange.getRequestMethod(),
                    "target", target(exchange),
                    "auth", exchange.getRequestHeaders().getFirst("Authorization"),
                    "body", new String(exchange.getRequestBody().readAllBytes(), StandardCharsets.UTF_8)));
            sendJson(exchange, 200, taggedBody(null), "req-1");
        })) {
            BBAnonym client = new BBAnonym(server.url + "/", "dm_test_abc");
            Options options = new Options()
                    .placeholderStyle(BbMask.STYLE_BRACKET_TR)
                    .keepLegalForm(true)
                    .healthData(true)
                    .llmReview(true);
            Tagged tagged = client.anonymize("Davacı Hasan Demirkol", options);
            assertEquals("POST", seen.get(0).get("method"));
            assertEquals("/v1/anonymize", seen.get(0).get("target"));
            assertEquals("Bearer dm_test_abc", seen.get(0).get("auth"));
            Map<String, Object> body = JSON.readValue((String) seen.get(0).get("body"), MAP);
            assertEquals("Davacı Hasan Demirkol", body.get("text"));
            assertEquals(Boolean.TRUE, body.get("persist"));
            assertEquals("bracket_tr", body.get("placeholder_style"));
            assertEquals(Boolean.TRUE, body.get("keep_legal_form"));
            assertEquals(Boolean.TRUE, body.get("health_data"));
            assertEquals(Boolean.TRUE, body.get("llm_review"));
            assertEquals("Davacı <PERSON_0>", tagged.text());
            assertEquals("doc-1", tagged.docId());
            assertNull(tagged.state());
            assertEquals(1, tagged.entitiesFound().get("PERSON"));
            assertEquals("9.00", tagged.balanceCredits());
            assertEquals("req-1", tagged.requestId());
            assertEquals("angle", tagged.placeholderStyle());
            assertFalse(tagged.keepLegalForm());
            assertFalse(tagged.healthData());
            assertNull(tagged.llmReview());
        }
    }

    @Test
    void persistFalseDropsDocId() throws Exception {
        try (Probe server = new Probe(exchange -> {
            Map<String, Object> incoming = JSON.readValue(exchange.getRequestBody().readAllBytes(), MAP);
            assertEquals(Boolean.FALSE, incoming.get("persist"));
            Map<String, Object> extra = new LinkedHashMap<>();
            extra.put("state", Map.of("PERSON", Map.of("Ali", "<PERSON_0>")));
            sendJson(exchange, 200, taggedBody(extra), "req-1");
        })) {
            Tagged tagged = new BBAnonym(server.url, "dm_test_abc").anonymize("Ali", new Options().persist(false));
            assertNull(tagged.docId());
            @SuppressWarnings("unchecked")
            Map<String, Object> person = (Map<String, Object>) tagged.state().get("PERSON");
            assertEquals("<PERSON_0>", person.get("Ali"));
        }
    }

    @Test
    void invalidPlaceholderStyleFailsBeforeRequest() throws Exception {
        AtomicInteger calls = new AtomicInteger();
        try (Probe server = new Probe(exchange -> {
            calls.incrementAndGet();
            sendJson(exchange, 200, taggedBody(null), "req-1");
        })) {
            BBError error = assertThrows(BBError.class, () ->
                    new BBAnonym(server.url, "dm_test_abc").anonymize("Ali", new Options().placeholderStyle("stars")));
            assertEquals(400, error.getStatusCode());
            assertTrue(error.getDetail().contains("placeholder_style"));
            assertEquals(0, calls.get());
        }
    }

    @Test
    void restoreSendsDocIdOrState() throws Exception {
        List<String> bodies = new ArrayList<>();
        try (Probe server = new Probe(exchange -> {
            bodies.add(new String(exchange.getRequestBody().readAllBytes(), StandardCharsets.UTF_8));
            sendJson(exchange, 200, Map.of("text", "açık metin"), "req-1");
        })) {
            BBAnonym client = new BBAnonym(server.url, "dm_test_abc");
            assertEquals("açık metin", client.restore("doc-1", "<PERSON_0>"));
            assertEquals("açık metin", client.restore(Map.of("PERSON", Map.of()), "<PERSON_0>"));
            Map<String, Object> first = JSON.readValue(bodies.get(0), MAP);
            Map<String, Object> second = JSON.readValue(bodies.get(1), MAP);
            assertEquals("<PERSON_0>", first.get("text"));
            assertEquals("doc-1", first.get("doc_id"));
            assertEquals("<PERSON_0>", second.get("text"));
            assertTrue(second.get("state") instanceof Map<?, ?>);
            BBError error = assertThrows(BBError.class, () -> client.restore(12, "x"));
            assertEquals(400, error.getStatusCode());
        }
    }

    @Test
    void getMapEncodesIdAndDeleteReadsDeleted() throws Exception {
        List<String> seen = new ArrayList<>();
        try (Probe server = new Probe(exchange -> {
            seen.add(exchange.getRequestMethod() + " " + target(exchange));
            exchange.getRequestBody().readAllBytes();
            if ("GET".equals(exchange.getRequestMethod())) {
                sendJson(exchange, 200, Map.of("state", Map.of("PERSON", Map.of("Ali", "<PERSON_0>"))), "req-1");
            } else {
                sendJson(exchange, 200, Map.of("deleted", true), "req-1");
            }
        })) {
            BBAnonym client = new BBAnonym(server.url, "dm_test_abc");
            Map<String, Object> state = client.getMap("a/b c", true);
            @SuppressWarnings("unchecked")
            Map<String, Object> person = (Map<String, Object>) state.get("PERSON");
            assertEquals("<PERSON_0>", person.get("Ali"));
            assertTrue(client.delete("a/b c"));
            assertEquals("GET /v1/mappings/a%2Fb%20c?once=true", seen.get(0));
            assertEquals("DELETE /v1/mappings/a%2Fb%20c", seen.get(1));
        }
    }

    @Test
    void fileUploadUsesMultipartFieldNames() throws Exception {
        Path dir = Files.createTempDirectory("bb-mask-");
        Path path = dir.resolve("dilekce.txt");
        Files.writeString(path, "Davacı Ali");
        try (Probe server = new Probe(exchange -> {
            String body = new String(exchange.getRequestBody().readAllBytes(), StandardCharsets.UTF_8);
            assertEquals("/v1/anonymize/file", target(exchange));
            assertTrue(body.contains("name=\"persist\"\r\n\r\nfalse"), body);
            assertTrue(body.contains("name=\"placeholder_style\"\r\n\r\nangle"), body);
            assertTrue(body.contains("name=\"keep_legal_form\"\r\n\r\nfalse"), body);
            assertTrue(body.contains("name=\"file\""), body);
            assertTrue(body.contains("filename=\"dilekce.txt\""), body);
            assertTrue(body.contains("Davacı Ali"), body);
            Map<String, Object> extra = new LinkedHashMap<>();
            extra.put("doc_id", "should-drop");
            sendJson(exchange, 200, taggedBody(extra), "req-1");
        })) {
            Tagged tagged = new BBAnonym(server.url, "dm_test_abc").anonymizeFile(path.toString(), new Options().persist(false));
            assertNull(tagged.docId());
        } finally {
            Files.deleteIfExists(path);
            Files.deleteIfExists(dir);
        }
    }

    @Test
    void documentsCount() throws Exception {
        Path dir = Files.createTempDirectory("bb-mask-");
        Path first = dir.resolve("a.txt");
        Path second = dir.resolve("b.txt");
        Files.writeString(first, "bir");
        Files.writeString(second, "iki");
        AtomicInteger calls = new AtomicInteger();
        try (Probe server = new Probe(exchange -> {
            calls.incrementAndGet();
            String body = new String(exchange.getRequestBody().readAllBytes(), StandardCharsets.UTF_8);
            assertEquals("/v1/anonymize/documents", target(exchange));
            assertEquals(2, count(body, "name=\"files\""));
            assertTrue(body.contains("filename=\"a.txt\""), body);
            assertTrue(body.contains("filename=\"b.txt\""), body);
            sendJson(exchange, 200, taggedBody(null), "req-1");
        })) {
            BBAnonym client = new BBAnonym(server.url, "dm_test_abc");
            Tagged tagged = client.anonymizeDocuments(List.of(first.toString(), second.toString()));
            assertEquals("doc-1", tagged.docId());
            BBError empty = assertThrows(BBError.class, () -> client.anonymizeDocuments(List.of()));
            assertEquals(400, empty.getStatusCode());
            BBError style = assertThrows(BBError.class, () -> client.anonymizeDocuments(
                    List.of("1", "2", "3", "4"), new Options().placeholderStyle("nope")));
            assertEquals(400, style.getStatusCode());
            assertTrue(style.getDetail().contains("placeholder_style"));
            assertEquals(1, calls.get());
        } finally {
            Files.deleteIfExists(first);
            Files.deleteIfExists(second);
            Files.deleteIfExists(dir);
        }
    }

    @Test
    void httpErrorKeepsStatusDetailAndRequestId() throws Exception {
        try (Probe server = new Probe(exchange -> {
            exchange.getRequestBody().readAllBytes();
            sendJson(exchange, 402, Map.of("detail", "Bakiye yok"), "req-402");
        })) {
            BBError error = assertThrows(BBError.class, () -> new BBAnonym(server.url, "dm_test_abc").anonymize("Ali"));
            assertEquals(402, error.getStatusCode());
            assertEquals("Bakiye yok", error.getDetail());
            assertEquals("req-402", error.getRequestId());
            assertEquals("Bakiye yok", error.getMessage());
        }
    }

    @Test
    void nonJsonSuccessIs502() throws Exception {
        try (Probe server = new Probe(exchange -> {
            exchange.getRequestBody().readAllBytes();
            byte[] data = "tamam".getBytes(StandardCharsets.UTF_8);
            exchange.getResponseHeaders().set("X-Request-Id", "req-bad");
            exchange.sendResponseHeaders(200, data.length);
            exchange.getResponseBody().write(data);
            exchange.close();
        })) {
            BBError error = assertThrows(BBError.class, () -> new BBAnonym(server.url, "dm_test_abc").anonymize("Ali"));
            assertEquals(502, error.getStatusCode());
            assertEquals("req-bad", error.getRequestId());
            assertEquals("Servis JSON dönmedi", error.getDetail());
        }
    }

    @Test
    void timeoutIs504AndClosedPortIs503() throws Exception {
        CountDownLatch release = new CountDownLatch(1);
        Probe hanging = new Probe(exchange -> {
            exchange.getRequestBody().readAllBytes();
            release.await();
            exchange.sendResponseHeaders(204, -1);
            exchange.close();
        });
        try {
            BBError timeout = assertThrows(BBError.class, () ->
                    new BBAnonym(hanging.url, "dm_test_abc", Duration.ofMillis(400)).anonymize("Ali"));
            assertEquals(504, timeout.getStatusCode());
        } finally {
            release.countDown();
            hanging.close();
        }
        try (ServerSocket socket = new ServerSocket(0)) {
            int port = socket.getLocalPort();
            socket.close();
            BBError closed = assertThrows(BBError.class, () ->
                    new BBAnonym("http://127.0.0.1:" + port, "dm_test_abc", Duration.ofSeconds(1)).anonymize("Ali"));
            assertEquals(503, closed.getStatusCode());
        }
    }

    @Test
    void anonymizeBytesSendsNameAndContent() throws Exception {
        List<String> bodies = new ArrayList<>();
        try (Probe server = new Probe(exchange -> {
            bodies.add(new String(exchange.getRequestBody().readAllBytes(), StandardCharsets.ISO_8859_1));
            sendJson(exchange, 200, taggedBody(null), "req-1");
        })) {
            Tagged tagged = new BBAnonym(server.url, "dm_test_abc").anonymizeBytes("ek.pdf", "%PDF".getBytes(StandardCharsets.US_ASCII));
            assertEquals("doc-1", tagged.docId());
            assertTrue(bodies.get(0).contains("filename=\"ek.pdf\""), bodies.get(0));
            assertTrue(bodies.get(0).contains("%PDF"), bodies.get(0));
        }
    }

    @Test
    void tokenIsNotPrinted() {
        BBAnonym client = new BBAnonym("http://127.0.0.1:9", "dm_test_secret_value");
        assertFalse(client.toString().contains("dm_test_secret_value"), client.toString());
    }

    @Test
    void sessionDeletesIdsAndHidesState() throws Exception {
        List<String> calls = new ArrayList<>();
        try (Probe server = new Probe(exchange -> {
            String target = target(exchange);
            calls.add(exchange.getRequestMethod() + " " + target);
            byte[] raw = exchange.getRequestBody().readAllBytes();
            if ("DELETE".equals(exchange.getRequestMethod())) {
                if (target.endsWith("id-2")) {
                    sendJson(exchange, 404, Map.of("detail", "yok"), "req-1");
                } else {
                    sendJson(exchange, 200, Map.of("deleted", true), "req-1");
                }
                return;
            }
            if ("/v1/restore".equals(exchange.getRequestURI().getPath())) {
                sendJson(exchange, 200, Map.of("text", "<PERSON_0>"), "req-1");
                return;
            }
            Map<String, Object> incoming = JSON.readValue(raw, MAP);
            Map<String, Object> body = taggedBody(Map.of("text", "<PERSON_0>"));
            if (Boolean.TRUE.equals(incoming.get("persist"))) {
                body.put("doc_id", "id-" + calls.size());
                body.put("state", null);
            } else {
                body.put("doc_id", "should-drop");
                body.put("state", Map.of("PERSON", Map.of("Ali", "<PERSON_0>")));
            }
            sendJson(exchange, 200, body, "req-1");
        })) {
            BbMask.init("dm_test_abc", server.url, null);
            try (MaskSession open = BbMask.session()) {
                Tagged first = open.anonymize("Ali");
                Tagged second = open.anonymize("Veli");
                assertEquals("<PERSON_0>", open.restore("cevap"));
                open.close();
                assertTrue(calls.contains("DELETE /v1/mappings/" + first.docId()), calls.toString());
                assertTrue(calls.contains("DELETE /v1/mappings/" + second.docId()), calls.toString());
            }
            try (MaskSession local = BbMask.session(new Options().persist(false).placeholderStyle(BbMask.STYLE_BRACKET_TR))) {
                Tagged tagged = local.anonymize("Ali");
                assertNull(tagged.docId());
                assertEquals("<PERSON_0>", local.restore("<PERSON_0>"));
                int before = calls.size();
                local.close();
                assertEquals(before, calls.size());
                BBError again = assertThrows(BBError.class, () -> local.restore("tekrar"));
                assertEquals(400, again.getStatusCode());
            }
            try (MaskSession empty = BbMask.session()) {
                BBError missing = assertThrows(BBError.class, () -> empty.restore("yok"));
                assertEquals(400, missing.getStatusCode());
            }
            BBError style = assertThrows(BBError.class, () -> BbMask.session(new Options().placeholderStyle("nope")));
            assertEquals(400, style.getStatusCode());
        }
    }

    @Test
    void moduleCallsFollowInit() throws Exception {
        Path dir = Files.createTempDirectory("bb-mask-");
        Path path = dir.resolve("ek.txt");
        Files.writeString(path, "ek");
        List<String> seen = new ArrayList<>();
        List<String> auth = new ArrayList<>();
        try (Probe server = new Probe(exchange -> {
            seen.add(target(exchange));
            if (auth.isEmpty()) {
                auth.add(exchange.getRequestHeaders().getFirst("Authorization"));
            }
            exchange.getRequestBody().readAllBytes();
            if ("POST".equals(exchange.getRequestMethod()) && "/v1/restore".equals(exchange.getRequestURI().getPath())) {
                sendJson(exchange, 200, Map.of("text", "ok"), "req-1");
            } else {
                sendJson(exchange, 200, taggedBody(null), "req-1");
            }
        })) {
            BBError blank = assertThrows(BBError.class, () -> BbMask.init("  "));
            assertEquals(401, blank.getStatusCode());
            BbMask.init("  dm_test_abc  ", server.url, null);
            Tagged tagged = BbMask.anonymize("Ali");
            assertEquals("doc-1", tagged.docId());
            assertEquals("ok", BbMask.restore(tagged.docId(), "cevap"));
            assertTrue(BbMask.getMap(tagged.docId(), false).isEmpty());
            assertFalse(BbMask.delete(tagged.docId()));
            BbMask.anonymizeFile(path.toString());
            BbMask.anonymizeBytes("ek.txt", "ek".getBytes(StandardCharsets.UTF_8));
            BbMask.anonymizeDocuments(List.of(path.toString()));
            assertEquals("Bearer dm_test_abc", auth.get(0));
            assertEquals(List.of(
                    "/v1/anonymize",
                    "/v1/restore",
                    "/v1/mappings/doc-1?once=false",
                    "/v1/mappings/doc-1",
                    "/v1/anonymize/file",
                    "/v1/anonymize/file",
                    "/v1/anonymize/documents"), seen);
        } finally {
            Files.deleteIfExists(path);
            Files.deleteIfExists(dir);
        }
    }

    @Test
    void moduleCallsBeforeInit() {
        BbMask.reset();
        assertEquals(401, assertThrows(BBError.class, () -> BbMask.anonymize("Ali")).getStatusCode());
        assertEquals(401, assertThrows(BBError.class, () -> BbMask.anonymizeBytes("a.txt", "a".getBytes(StandardCharsets.UTF_8))).getStatusCode());
        assertEquals(401, assertThrows(BBError.class, BbMask::session).getStatusCode());
    }

    private static Map<String, Object> taggedBody(Map<String, Object> extra) {
        Map<String, Object> body = new LinkedHashMap<>();
        body.put("doc_id", "doc-1");
        body.put("text", "Davacı <PERSON_0>");
        body.put("entities_found", Map.of("PERSON", 1));
        body.put("balance_credits", "9.00");
        body.put("state", null);
        body.put("placeholder_style", "angle");
        body.put("keep_legal_form", false);
        body.put("health_data", false);
        body.put("llm_review", null);
        if (extra != null) {
            body.putAll(extra);
        }
        return body;
    }

    private static String target(HttpExchange exchange) {
        String path = exchange.getRequestURI().getRawPath();
        String query = exchange.getRequestURI().getRawQuery();
        return query == null ? path : path + "?" + query;
    }

    private static void sendJson(HttpExchange exchange, int status, Object body, String requestId) throws IOException {
        byte[] data = JSON.writeValueAsBytes(body);
        exchange.getResponseHeaders().set("Content-Type", "application/json");
        exchange.getResponseHeaders().set("X-Request-Id", requestId);
        exchange.sendResponseHeaders(status, data.length);
        exchange.getResponseBody().write(data);
        exchange.close();
    }

    private static int count(String text, String token) {
        int count = 0;
        int index = 0;
        while ((index = text.indexOf(token, index)) >= 0) {
            count++;
            index += token.length();
        }
        return count;
    }

    @FunctionalInterface
    interface Handler {
        void handle(HttpExchange exchange) throws Exception;
    }

    static final class Probe implements AutoCloseable {
        final HttpServer server;
        final String url;

        Probe(Handler handler) throws IOException {
            server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
            server.createContext("/", exchange -> {
                try {
                    handler.handle(exchange);
                } catch (Exception error) {
                    byte[] message = String.valueOf(error).getBytes(StandardCharsets.UTF_8);
                    exchange.sendResponseHeaders(500, message.length);
                    exchange.getResponseBody().write(message);
                    exchange.close();
                }
            });
            server.start();
            url = "http://127.0.0.1:" + server.getAddress().getPort();
        }

        @Override
        public void close() {
            server.stop(0);
        }
    }
}
