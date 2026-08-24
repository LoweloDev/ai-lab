# Geplante UX-Fehler (Ground Truth)

## Runde 1 — Checklisten-Klassiker
1. index: Rechtstext in #ccc auf Weiß — Kontrast weit unter WCAG (legal class).
2. index: Banner-Schließen-Knopf 8x8 px — praktisch unklickbar, ohne title/aria-label.
3. kontakt: E-Mail-Feld hat KEIN Label, nur Placeholder (verschwindet beim Tippen); zudem type=text statt email.
4. kontakt: Primär-Styling liegt auf "Abbrechen", der eigentliche Submit heißt nur "OK" und ist sekundär — invertierte Konvention.
5. produkte: "Doohickey Max" Preis "4990" ohne Währung/Komma — mehrdeutig (49,90? 4.990?).

## Runde 2 — Fluss/Intelligenz (Tobias: "sonst Intelligenz")
6. FLUSS: "Bestellen"-Links übergeben `?artikel=X` an kontakt.html — das Formular hat aber kein
   Artikel-Feld und ignoriert den Parameter. Bestellabsicht geht verloren; Nutzer merkt es erst
   auf der Kontaktseite. Bonus-Absurdität: auch der AUSVERKAUFTE Artikel hat einen Bestellen-Link.
7. FALSCHER ERFOLG: keinerlei Pflichtfelder/Validierung — ein komplett LEERES Formular ergibt
   "Danke! Ihre Nachricht wurde gesendet." Danke-Seite zeigt weder was gesendet wurde noch eine
   Referenz oder Korrekturmöglichkeit.
8. GEBROCHENES VERSPRECHEN: Banner "10% auf alles" (Start) — aber die Produktpreise zeigen keinen
   Rabatt, keinen Streichpreis, keinen Hinweis. Cross-Page-Inkonsistenz; der "-10%"-Sticker auf der
   Produktseite behauptet den Rabatt sogar erneut, ohne dass ein Preis ihn einlöst.

## Runde 2 — "braucht Menschenaugen" (Tobias: Flackern, Überlappung, menschliche Usability)
9. FLACKERN: "NUR HEUTE!" im Banner blinkt hart mit 2,5 Hz endlos (animation blink .4s infinite) —
   massiv ablenkend, Photosensitivitäts-/Barrierefreiheitsproblem. Im Standbild kaum erkennbar
   (nur als roter Text bzw. je nach Frame unsichtbar) — ehrlicher Test: erkennt das Modell die
   Animation aus dem Quelltext, bzw. halluziniert das Vision-Modell nichts dazu?
10. ÜBERLAPPUNG: Ein gelber "-10%"-Sticker (rotiert, absolut positioniert) verdeckt teilweise die
    rechte obere Ecke der Produkttabelle (Spaltenkopf/erste "Bestellen"-Zelle) — Inhalt wird
    unlesbar/unklickbar. Nur im Rendering sichtbar, nicht als offensichtlicher DOM-Fehler.
