# UX-Wertung gegen die Ground Truth

Grundlage: `UX-FLAWS.md` (10 gepflanzte Fehler) gegen alle Reviews in `runs-ux/`.
Klassen: **Klassiker** = #1–#5, **Fluss** = #6–#8, **Menschenaugen** = #9–#10.

Wertungsregel: ein Fehler zählt nur, wenn das Review das *konkrete* Problem an der richtigen
Stelle benennt, nicht bloß die Kategorie. Im Zweifel nicht gezählt.
**Bonus** = reales, nicht gepflanztes Problem (häufigster Fall: aktiver „Bestellen"-Link am
ausverkauften Artikel — laut Ground Truth ausdrücklich kein Teil von #6).
**Falsch** = nachweislich unzutreffender oder halluzinierter Befund.
Befunde, die einen gepflanzten Fehler nur streifen (z. B. „OK" als vage Beschriftung ohne die
vertauschte Primär-Hierarchie), zählen weder als Treffer noch als Bonus noch als Falsch.

## DOM-Pfad (Quelltext von index/produkte/kontakt, ohne danke.html)

| Label | Klassiker (x/5) | Fluss (x/3) | Menschenaugen (x/2) | Gesamt (x/10) | Bonus | Falsch | Dauer |
|---|---|---|---|---|---|---|---|
| cc-opus5 | 5 | 3 | 2 | **10** | 9 | 0 | 135,8 s |
| cc-opus48 | 5 | 3 | 2 | **10** | 3 | 0 | 66,4 s |
| muse-dom | 5 | 2 | 1 | **8** | 3 | 0 | 67,3 s |
| gemini37f | 5 | 1 | 1 | **7** | 3 | 0 | 10,5 s |
| ds-pro | 4 | 2 | 1 | **7** | 3 | 1 | 109,0 s |
| agy-37flash | 5 | 1 | 1 | **7** | 1 | 0 | 19,9 s |
| qwen36moe | 4 | 1 | 1 | **6** | 3 | 0 | 38,6 s |
| codernext | 3 | 1 | 1 | **5** | 5 | 3 | 60,2 s |
| ds-flash | 0 | 0 | 0 | **0** | 0 | 0 | 165,3 s |
| qwen38-dom | 0 | 0 | 0 | **0** | 0 | 0 | 175,8 s |

- **cc-opus5** — verfehlt: keine (10/10)
- **cc-opus48** — verfehlt: keine (10/10)
- **muse-dom** — verfehlt: #8 (Rabattversprechen ohne Preis-Einlösung), #10 (Sticker-Überlappung)
- **gemini37f** — verfehlt: #7 (leeres Formular ohne Validierung), #8, #10
- **ds-pro** — verfehlt: #4 (invertierte Button-Hierarchie; nennt nur „OK" als vage), #7, #10
- **agy-37flash** — verfehlt: #7, #8, #10
- **qwen36moe** — verfehlt: #4, #7, #8, #10
- **codernext** — verfehlt: #1 (Kontrast Rechtstext), #4, #6 (verlorener `?artikel`-Parameter), #8, #10
- **ds-flash** — verfehlt: #1–#10 (leere Ausgabe trotz 165,3 s)
- **qwen38-dom** — verfehlt: #1–#10 (leere Ausgabe trotz 175,8 s)

## Vision-Pfad (Screenshots; nur Qwen3.8 + Muse)

Runde 1 (`muse.md`, `qwen38.md`): drei Screenshots. Runde 2 (`*-r2.md`): zusätzlich der zweite
Startseiten-Frame (`index-b.png`) und die Danke-Seite.

| Label | Klassiker (x/5) | Fluss (x/3) | Menschenaugen (x/2) | Gesamt (x/10) | Bonus | Falsch | Dauer |
|---|---|---|---|---|---|---|---|
| qwen38 | 5 | 0 | 0 | **5** | 3 | 1 | 127,5 s |
| muse | 5 | 0 | 0 | **5** | 1 | 1 | 96,5 s |
| qwen38-r2 | 1 | 0 | 1 | **2** | 0 | 0 | 156,4 s |
| muse-r2 | 0 | 0 | 0 | **0** | 0 | 0 | 76,4 s |

- **qwen38** — verfehlt: #6, #7, #8, #9, #10. Falsch: „Produktseite — keine Kaufmöglichkeit,
  weder Warenkorb-Buttons noch Produkt-Links" — die vier „Bestellen"-Links stehen sichtbar in der
  Tabelle.
- **muse** — verfehlt: #6, #7, #8, #9, #10. Falsch: derselbe Befund („die Liste ist eine Sackgasse").
- **qwen38-r2** — verfehlt: #1, #3, #4, #5, #6, #7, #8, #10. Erkennt aus den zwei Frames als
  einziges Vision-Review das Flackern (#9) und den winzigen Schließen-Knopf (#2), bricht dann
  mitten im Satz ab.
- **muse-r2** — verfehlt: #1–#10 (leere Ausgabe)

## Muster

Die Checklisten-Klassiker trennen niemanden mehr: sechs der acht nicht-leeren DOM-Reviews holen
5/5, die beiden Vision-Läufe aus Runde 1 ebenfalls — Kontrast, Trefferfläche, fehlendes Label,
falscher Input-Typ und der unformatierte Preis werden von jedem Modell gefunden, das überhaupt
antwortet. Getrennt wird ausschließlich in den beiden anderen Klassen, und zwar nicht graduell,
sondern binär.

**Fluss (#6–#8)** ist die erste Bruchlinie. #6 (der `?artikel`-Parameter, den das Formular
ignoriert) verlangt, zwei Dateien gegeneinander zu lesen — sieben von acht DOM-Reviews schaffen
das, codernext nicht. #7 (kein `required`, leeres Formular geht durch) schaffen nur vier; die
übrigen bleiben bei „fehlende Pflichtfeld-Kennzeichnung" stehen und unterstellen dabei implizit
eine Validierung, die es nicht gibt. #8 (Banner verspricht 10 %, kein Preis löst das ein) ist der
härteste Einzelfehler des Sets: nur cc-opus5, cc-opus48 und ds-pro benennen, dass die Preise den
Rabatt nirgends abbilden — alle anderen sehen den Sticker, fragen aber nur, *worauf* er sich
bezieht, statt zu prüfen, *ob* ihn ein Preis einlöst.

**Menschenaugen (#9–#10)** ist die zweite und schärfere. #9 (Flackern) ist im DOM-Pfad trivial —
`animation:blink .4s infinite` steht im Quelltext, acht von acht finden es; im Vision-Pfad findet
es nur qwen38-r2, und zwar erst, als zwei Frames derselben Seite vorlagen. #10 (der Sticker, der
die rechte obere Tabellenecke verdeckt) ist der einzige Fehler, den nur die beiden Claude-Läufe
haben: er verlangt, aus `position:absolute; top:-6px; right:-14px; z-index:2` die tatsächliche
Geometrie im Kopf zu rendern. Alle anderen erwähnen den Sticker, keiner rechnet ihn aus.

Der Vision-Pfad kehrt das Bild nicht um, sondern verschlechtert es: 0/3 im Fluss und ein
identischer Halluzinationsfehler bei beiden Modellen — beide behaupten, die Produkttabelle biete
keinerlei Kaufmöglichkeit, obwohl vier „Bestellen"-Links deutlich sichtbar sind. Der Sticker
verdeckt genau die Kopfzelle dieser Spalte; die Modelle haben die Spalte daraufhin komplett
verworfen, statt die Überlappung als Befund zu melden. Damit produziert #10 im Vision-Pfad
ausgerechnet einen falschen Befund statt eines Treffers.

Nebenbefund zur Zuverlässigkeit: ds-flash und qwen38-dom liefern nach 165 bzw. 176 Sekunden eine
komplett leere Antwort — die beiden langsamsten DOM-Läufe der Kampagne sind zugleich die einzigen
ohne jeden Inhalt. Dauer sagt hier nichts über Qualität: gemini37f holt 7/10 in 10,5 Sekunden,
cc-opus5 holt 10/10 plus neun Bonusbefunde in 135,8 Sekunden.

Bonusverhalten trennt zusätzlich, aber in beide Richtungen: der aktive „Bestellen"-Link am
ausverkauften Artikel wird von sieben der acht nicht-leeren DOM-Reviews gefunden und ist damit der
verlässlichste Zusatzfund. codernext dagegen kauft seine fünf Bonusbefunde mit drei falschen ein
(erfundener `name`/`id`-Konflikt am Textarea, `type="button"` verhindere Tastaturbedienung,
fehlendes `tabindex` an einem rein dekorativen Sticker) — mehr Befunde bedeuten dort nicht mehr
Substanz, sondern mehr Rauschen.
