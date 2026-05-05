---
id: currency
title: Currency
keywords: money, coins, gold, silver, copper
---
WheelMUD uses a four-tier coin system:

  cp  copper
  sp  silver  (10 cp)
  gp  gold    (10 sp = 100 cp)
  pp  platinum (10 gp = 1000 cp)

World data and shop prices express amounts as compact strings
(e.g. {{2gp 5sp}}::yellow), parsed by internal/currency. A future
{{shop}}::cyan command (ROADMAP.md §14) will surface item values
inline; until then, examine an item to see its declared worth.

Banks and vaults are also pending — for now coins live only in
character inventories.
