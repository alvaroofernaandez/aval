# DESIGN.md schema

Every project keeps `.agents/DESIGN.md`. It is binding: if output conflicts with
it, the output is wrong. If the user asks for something that conflicts, surface
the conflict instead of silently picking a side: either the decision is being
changed on purpose (log it in section 8 with its reason) or the request is out
of scope.

Create it the first time UI work happens in a project. Update it in the same
change that introduces a decision. Mark superseded entries, never delete them.
`aval design ack` records its sha1, so editing it invalidates earlier acks.

```
# DESIGN.md — <Project Name>

## 1. Intent & North Star
<art direction, voice, anti-references>

## 2. Tokens
Colour (hex + semantic role) · Typography (family, size, weight, line-height)
· Spacing · Radius · Shadow/elevation · Z-index layers

## 3. Layout Primitives
Grid, breakpoints, container widths

## 4. Component Patterns
Per component: purpose, anatomy, states, accessibility notes, code reference

## 5. Motion System
Curves, durations, choreography, prefers-reduced-motion strategy

## 6. Accessibility Baselines
WCAG level, contrast minimums, focus rules, keyboard rules

## 7. Anti-patterns (banned here, and why)

## 8. Decision Log
Dated: decision, rationale, supersedes
```
