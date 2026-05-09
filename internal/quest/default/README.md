# Default quest catalog

Drop one YAML file per quest in this directory; the embedded loader
walks every `*.yaml` file at boot and assembles `quest.Catalog`.
Override the embedded set with the `QUEST_DIR` environment variable
to point at a directory of YAML files outside the binary.

Schema (one file per quest):

```yaml
id: lost_lamb
name: The Lost Lamb
summary: A village elder's lamb has wandered off.
giver_mob: tr.elder
steps:
  - kind: reach_room
    room: tr.westwood.path_2
    prompt: Search the Westwood path for the missing lamb.
  - kind: kill_n
    mob: tr.westwood.wolf
    count: 3
    prompt: Drive off the wolves stalking the path.
  - kind: talk_to
    mob: tr.elder
    prompt: Return to the village elder.
rewards:
  xp: 200
  copper: 5000
```

Step kinds (V1):

- `talk_to` requires `mob` (a mob_template ExternalID); advances when
  the player reaches that NPC and the dialogue tree fires an
  `advance_quest` effect targeting this quest.
- `kill_n` requires `mob` (a mob_template ExternalID) and `count > 0`;
  decrements per kill. Per-step state is `{"remaining": N}`.
- `reach_room` requires `room` (a room ExternalID); advances on the
  next `world.PlayerEntered` event matching that room.

Cross-reference validation runs at boot (after the world loader
populates rooms + mob_templates). A typo in `mob` or `room` fails
the boot loudly with the offending ID in the error.
