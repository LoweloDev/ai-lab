# agy-37flash-low — Token-Bilanz Suite (8 Tasks, 25.08. 02:39–02:58)

| Task | Input | Output | davon Thinking | Cache-Reads | total |
|---|---|---|---|---|---|
| agora-A1-gate | 162,289 | 3,341 | 1,511 | 190,650 | 165,630 |
| agora-A2-jsonld | 190,292 | 5,012 | 2,285 | 255,301 | 195,304 |
| agora-A3-hls | 187,656 | 5,465 | 1,400 | 524,023 | 193,121 |
| agora-A4-feed | 136,768 | 2,188 | 0 | 304,403 | 138,956 |
| agora-A5-batcher-scratch | 234,130 | 9,451 | 0 | 1,311,482 | 243,581 |
| agora-A6-scorer-scratch | 315,261 | 13,071 | 0 | 2,834,313 | 328,332 |
| aiux-U1-paging | 177,422 | 3,416 | 1,385 | 365,256 | 180,838 |
| aiux-U2-denytools | 496,160 | 11,512 | 0 | 6,553,885 | 507,672 |
| **gesamt** | **1,899,978** | **53,456** | 6,581 | 12,339,313 | **1,953,434** |

Vergleich high (Suite 8 Tasks, 24.08.): Input 2.091.183 · Output 169.773 · Thinking 108.930 · Cache 14.539.411 · total 2.260.956
Ergebnis: low 7/8 = high 7/8 (beide nur A5 rot, gleiche Property). Zeit: low 481 s vs high 907 s (Summe).
Output 3,2x weniger, Thinking 16x weniger, Input gleich. → high bringt in dieser Suite keinen zusaetzlichen PASS.
