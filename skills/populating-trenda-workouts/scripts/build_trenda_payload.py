#!/usr/bin/env python3
import argparse
import html
import json
import sys


SET_KEYS = [
    ("reps", "Reps"),
    ("kg", "Kilograms"),
    ("lb", "Pounds"),
    ("meters", "Meters"),
    ("km", "Kilometers"),
    ("cal", "Calories"),
    ("watts", "Watts"),
    ("seconds", "Seconds"),
    ("minutes", "Minutes"),
    ("hours", "Hours"),
    ("circles", "Circles"),
    ("quantity", "Quantity"),
]

SECTIONS = ("warmup", "workout", "cooldown")


def load_json(path):
    if path == "-":
        return json.load(sys.stdin)
    with open(path, "r", encoding="utf-8") as fh:
        return json.load(fh)


def html_lines(lines):
    safe = [html.escape(str(line), quote=False) for line in lines if str(line).strip()]
    return f"<p>{'<br>'.join(safe)}</p>" if safe else ""


def compact(obj):
    if isinstance(obj, dict):
        out = {}
        for key, value in obj.items():
            if value in (None, "", [], {}):
                continue
            out[key] = compact(value)
        return out
    if isinstance(obj, list):
        return [compact(item) for item in obj]
    return obj


def text_values(obj):
    if isinstance(obj, dict):
        for value in obj.values():
            yield from text_values(value)
    elif isinstance(obj, list):
        for value in obj:
            yield from text_values(value)
    elif isinstance(obj, str):
        yield obj


def build_sets(raw_sets):
    out = []
    for idx, raw in enumerate(raw_sets or [], start=1):
        known = {key for key, _ in SET_KEYS}
        unknown = sorted(set(raw) - known - {"order", "status"})
        if unknown:
            raise ValueError(
                "неизвестные ключи подхода: " + ", ".join(unknown)
                + ". Допустимы: " + ", ".join(key for key, _ in SET_KEYS)
            )
        used = []
        for source_key, unit in SET_KEYS:
            if source_key in raw and raw[source_key] is not None:
                used.append((source_key, unit, raw[source_key]))
        if not used:
            raise ValueError(
                "подход без нагрузки: укажите хотя бы одну единицу "
                + "(" + ", ".join(key for key, _ in SET_KEYS) + ")"
            )

        entry = {"order": int(raw.get("order", idx)), "primary_unit": used[0][1], "target_primary_unit": used[0][2], "status": raw.get("status", "None")}
        if len(used) >= 2:
            entry["secondary_unit"] = used[1][1]
            entry["target_secondary_unit"] = used[1][2]
        if len(used) >= 3:
            entry["tertiary_unit"] = used[2][1]
            entry["target_tertiary_unit"] = used[2][2]
        out.append(entry)
    return out


def exercise_type(section_name, kind):
    if kind == "mixed_modal":
        return "MixedModal"
    if section_name == "warmup":
        return "Warmup"
    if section_name == "cooldown":
        return "Cooldown"
    return "Exercise"


def build_exercise(item, section_name, sort_order):
    kind = item["kind"]
    ex_type = exercise_type(section_name, kind)
    exercise = {
        "type": ex_type,
        "title": item.get("title", ""),
        "sort_order": int(item.get("sort_order", sort_order)),
        "sets": build_sets(item.get("sets")),
    }

    if item.get("exercise_id") is not None:
        exercise["exercise_id"] = int(item["exercise_id"])

    if item.get("tempo"):
        exercise["tempo"] = str(item["tempo"])
    if item.get("both_sides") is not None:
        exercise["both_sides"] = bool(item["both_sides"])
    if item.get("video_urls"):
        exercise["video_urls"] = list(item["video_urls"])

    if kind == "mixed_modal":
        exercise["complex_details"] = html_lines(item.get("lines", []))
        instructions = item.get("instructions_lines") or item.get("description_lines") or []
        if instructions:
            exercise["description"] = html_lines(instructions)
    else:
        lines = item.get("description_lines") or item.get("lines") or []
        if lines:
            exercise["description"] = html_lines(lines)

    return compact(exercise)


def build_text_block(item, section_name, block_order):
    title = item.get("title", "")
    exercise = {
        "type": {"warmup": "Warmup", "cooldown": "Cooldown"}.get(section_name, "Exercise"),
        "title": title,
        "sort_order": 0,
        "description": html_lines(item.get("lines", [])),
        "sets": [],
    }
    if item.get("video_urls"):
        exercise["video_urls"] = list(item["video_urls"])
    compacted_exercise = compact(exercise)
    compacted_exercise["title"] = title
    return {
        "is_superset": False,
        "sort_order": block_order,
        "exercises": [compacted_exercise],
    }


def build_block(item, section_name, block_order):
    kind = item["kind"]
    if kind == "text":
        return build_text_block(item, section_name, block_order)
    if kind == "superset":
        for child in item.get("exercises", []):
            child_kind = child.get("kind")
            if child_kind not in {"exercise", "mixed_modal"}:
                raise ValueError(
                    "внутри superset допустимы только kind exercise и mixed_modal, "
                    f"получено: {child_kind!r}"
                )
        exercises = [
            build_exercise(exercise, section_name, exercise_order)
            for exercise_order, exercise in enumerate(item.get("exercises", []))
        ]
        if not exercises:
            raise ValueError("superset item requires non-empty exercises")
        return {"is_superset": True, "sort_order": block_order, "exercises": exercises}
    if kind in {"exercise", "mixed_modal"}:
        exercise = build_exercise(item, section_name, 0)
        return {"is_superset": False, "sort_order": block_order, "exercises": [exercise]}
    raise ValueError(f"unsupported item kind: {kind}")


def build_section(items, section_name):
    return [build_block(item, section_name, idx) for idx, item in enumerate(items or [])]


def build_payload(plan, mode):
    # В режиме update-body название дня не трогаем, если его не задали явно:
    # раньше оно всегда перезаписывалось пустой строкой и стиралось у клиента.
    payload = {
        "title": plan.get("title", ""),
        "description": plan.get("description", ""),
        "warmup": build_section(plan.get("warmup"), "warmup"),
        "workout": build_section(plan.get("workout"), "workout"),
        "cooldown": build_section(plan.get("cooldown"), "cooldown"),
    }

    if mode == "create":
        payload["client_id"] = int(plan["client_id"])
        payload["date"] = plan["date"]
        payload["type"] = plan.get("type", "Workout")
        if "sort_order" in plan:
            payload["sort_order"] = int(plan["sort_order"])
    elif mode == "update-body":
        payload["id"] = int(plan["workout_id"])
    else:
        raise ValueError(f"unsupported mode: {mode}")

    return payload


def validate_section_item(item, section_name):
    kind = item.get("kind")
    if kind not in {"text", "exercise", "mixed_modal", "superset"}:
        raise ValueError(
            f"элемент секции {section_name} требует kind из "
            + ", ".join(sorted({"text", "exercise", "mixed_modal", "superset"}))
            + f", получено: {kind!r}"
        )
    if kind == "text" and section_name in {"warmup", "cooldown"}:
        if str(item.get("title", "")).strip():
            raise ValueError(
                f"{section_name} text block title must be empty; "
                "Trenda provides the base section heading"
            )
    if kind == "mixed_modal":
        if not str(item.get("title", "")).strip():
            raise ValueError("mixed_modal item requires the source workout title")
        if not item.get("lines"):
            raise ValueError("mixed_modal item requires non-empty composition lines")
        if item.get("exercise_id") is not None:
            raise ValueError("mixed_modal item must not reference a library exercise_id")

    if kind == "superset":
        for child in item.get("exercises", []):
            validate_section_item(child, section_name)


def validate_plan(plan, mode):
    missing = [section for section in SECTIONS if section not in plan]
    if missing:
        raise ValueError("plan is missing sections: " + ", ".join(missing))
    # Название дня по умолчанию пустое: так его оставляет и само приложение.
    # Запрещены только метки источника (трек, уровень, BTWB), а осознанно
    # заданное пользователем название пропускаем по флагу allow_title.
    title = str(plan.get("title", "")).strip()
    if title and not plan.get("allow_title", False):
        raise ValueError(
            "top-level Trenda workout title must be empty; "
            "set allow_title: true only when the user explicitly asked for a title"
        )
    if not plan.get("allow_btwb_attribution", False):
        if any("btwb" in value.lower() for value in text_values(plan)):
            raise ValueError(
                "BTWB attribution and links are not allowed without an explicit instruction"
            )
    for section in SECTIONS:
        for item in plan.get(section, []):
            validate_section_item(item, section)
    if mode == "create":
        for field in ("client_id", "date"):
            if field not in plan:
                raise ValueError(f"plan is missing required field for create mode: {field}")
    if mode == "update-body" and "workout_id" not in plan:
        raise ValueError("plan is missing required field for update-body mode: workout_id")


def main():
    parser = argparse.ArgumentParser(description="Build strict Trenda request payloads from a simpler normalized workout plan.")
    parser.add_argument("--input", required=True, help="Path to normalized plan JSON, or - for stdin")
    parser.add_argument("--mode", choices=["create", "update-body"], required=True, help="Which Trenda request body to build")
    args = parser.parse_args()

    plan = load_json(args.input)
    validate_plan(plan, args.mode)
    payload = build_payload(plan, args.mode)
    json.dump(payload, sys.stdout, ensure_ascii=True, indent=2)
    sys.stdout.write("\n")


if __name__ == "__main__":
    main()
