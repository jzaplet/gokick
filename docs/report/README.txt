gokick — technické hodnocení (zdroj reportu)
============================================

Z těchto souborů se generuje ../gokick-hodnoceni.pdf (odkazovaný z gokick-roadmap.md).

Soubory
-------
  report.html      Šablona reportu (HTML + inline CSS). Zde se edituje obsah/design.
                   Cover obrázek je zástupný symbol __MASCOT_SRC__, který build vyplní.
  mascot_card.png  Hotový cover obrázek (měkký stín + bílá karta + maskot, vše v pixelech).
  build.sh         Vyrobí PDF: vloží mascot_card.png do šablony a vyrenderuje přes Chrome.
  make-cover.py    Přegeneruje mascot_card.png z ../go-vue-cqrs-ddd.png (jen když se mění maskot; vyžaduje Pillow).

Jak vyrobit PDF (běžný případ)
------------------------------
  cd docs/report
  ./build.sh
  # → zapíše ../gokick-hodnoceni.pdf

Vyžaduje: Google Chrome + python3 (stdlib). Cesta k Chrome jde přebít: CHROME=/cesta ./build.sh

Když se mění maskot (cover)
---------------------------
  pip install Pillow
  python3 make-cover.py     # přepíše mascot_card.png
  ./build.sh

Poznámky
--------
- Stín je záměrně „zapečený" do mascot_card.png. CSS box-shadow se v PDF prohlížečích
  renderuje nespolehlivě (často jako ostrý rámeček), rastr vypadá všude stejně.
- report_final.html je dočasný build artefakt (build.sh ho po renderu maže).
- Skóre, texty a kapitoly se mění přímo v report.html (hledej sekce <!-- ===== ... ===== -->).
