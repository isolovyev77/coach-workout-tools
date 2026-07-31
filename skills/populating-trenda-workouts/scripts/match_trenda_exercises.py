#!/usr/bin/env python3
import argparse
import json
import math
import re
import sys
from difflib import SequenceMatcher
from functools import lru_cache


ALIASES = {
    "du": "double under",
    "double unders": "double under",
    "single unders": "single under",
    "t2b": "toes to bar",
    "ttb": "toes to bar",
    "c2b": "chest to bar pull up",
    "hspu": "handstand push up",
    "rmu": "ring muscle up",
    "bmu": "bar muscle up",
    "fs": "front squat",
    "bs": "back squat",
    "pc": "power clean",
    "pp": "push press",
    "push jerk": "jerk",
    "airbike": "assault bike",
    "echo bike": "assault bike",
    "bike erg": "bike erg",
    "ski erg": "ski erg",
    "row erg": "row erg",
    "db": "dumbbell",
    "kb": "kettlebell",
    "bb": "barbell",
    "гири": "kettlebell",
    "гиря": "kettlebell",
    "гантели": "dumbbell",
    "гантель": "dumbbell",
    "штанга": "barbell",
    "турник": "pull up bar",
    "подтягивания": "pull up",
    "подтягивание": "pull up",
    "отжимания": "push up",
    "бёрпи": "burpee",
    "берпи": "burpee",
    "присед": "squat",
    "приседания": "squat",
    "становая": "deadlift",
    "толчок": "jerk",
    "жимовой швунг": "push press",
    "рывок": "snatch",
    "взятие": "clean",
    "взятие на грудь": "clean",
    "выход силой": "muscle up",
    "ходьба на руках": "handstand walk",
    "гребля": "row erg",
    "гребной тренажер": "row erg",
    "лыжный эргометр": "ski erg",
}

MOVEMENTS = {
    "squat", "deadlift", "clean", "snatch", "jerk", "press", "push press",
    "bench press", "pull up", "chin up", "row", "row erg", "ski erg",
    "assault bike", "bike erg", "toes to bar", "double under", "single under",
    "burpee", "lunge", "thruster", "wall ball", "handstand push up",
    "handstand walk", "muscle up", "ring dip", "push up", "sit up", "run",
    "carry", "swing", "box jump", "step up", "rope climb", "bike", "plank",
}

IMPLEMENTS = {
    "barbell", "dumbbell", "kettlebell", "bodyweight", "machine", "band",
    "ring", "rope", "box", "sled", "row erg", "ski erg", "bike erg",
    "assault bike", "pull up bar", "landmine", "cable", "medicine ball",
}

MODIFIERS = {
    "front", "back", "overhead", "power", "squat", "hang", "strict",
    "kipping", "butterfly", "single arm", "single leg", "alternating",
    "paused", "pause", "tempo", "deficit", "romanian", "sumo", "incline",
    "decline", "seated", "standing", "walking", "chest to bar", "ring",
    "bar", "floor", "block",
}

TOKEN_RE = re.compile(r"[a-z0-9]+")


def load_json(path):
    if path == "-":
        return json.load(sys.stdin)
    with open(path, "r", encoding="utf-8") as fh:
        return json.load(fh)


# Подстановки компилируются один раз: раньше список пересортировывался и
# перекомпилировался на каждый вызов, а вызовов на одну библиотеку тысячи.
_ALIAS_RULES = [
    (re.compile(rf"\b{re.escape(src)}\b"), dst)
    for src, dst in sorted(ALIASES.items(), key=lambda item: len(item[0]), reverse=True)
]
_WS_RE = re.compile(r"\s+")


@lru_cache(maxsize=None)
def canon_phrase(text):
    lowered = text.lower().replace("ё", "е")
    lowered = lowered.replace("/", " ").replace("-", " ")
    for pattern, dst in _ALIAS_RULES:
        lowered = pattern.sub(dst, lowered)
    return _WS_RE.sub(" ", lowered).strip()


@lru_cache(maxsize=None)
def _tokens_cached(text):
    return tuple(TOKEN_RE.findall(canon_phrase(text)))


def tokens(text):
    return _tokens_cached(text)


def phrase_flags(text, universe):
    expanded = canon_phrase(text)
    found = set()
    for phrase in universe:
        if phrase in expanded:
            found.add(phrase)
    return found


def combined_name(model):
    parts = [model.get("name"), model.get("name_default_lng"), model.get("name_second_lng")]
    return " | ".join(str(p) for p in parts if p)


def name_variants(model):
    """Каждое название по отдельности, включая обе стороны двуязычного "рус / eng".

    Лексическое сходство считается по лучшему варианту: у библиотеки Trenda имена
    вида "Фронтальные приседания / Front Squats", и сравнение со склеенной строкой
    занижало оценку примерно на 0.35, из-за чего точные совпадения получали вердикт
    manual вместо reuse_library.
    """
    seen = []
    for part in (model.get("name"), model.get("name_default_lng"), model.get("name_second_lng")):
        if not part:
            continue
        text = str(part)
        for piece in [text] + text.split("/"):
            piece = piece.strip()
            if piece and piece not in seen:
                seen.append(piece)
    return seen or [""]


def score_candidate(query, model):
    query_text = canon_phrase(query)
    cand_text = canon_phrase(combined_name(model))
    query_tokens = set(tokens(query))

    token_jaccard = 0.0
    ratio = 0.0
    for variant in name_variants(model):
        variant_text = canon_phrase(variant)
        variant_tokens = set(tokens(variant_text))
        overlap = len(query_tokens & variant_tokens)
        denom = max(len(query_tokens | variant_tokens), 1)
        token_jaccard = max(token_jaccard, overlap / denom)
        ratio = max(ratio, SequenceMatcher(None, query_text, variant_text).ratio())

    query_moves = phrase_flags(query, MOVEMENTS)
    cand_moves = phrase_flags(cand_text, MOVEMENTS)
    query_impl = phrase_flags(query, IMPLEMENTS)
    cand_impl = phrase_flags(cand_text, IMPLEMENTS)
    query_mod = phrase_flags(query, MODIFIERS)
    cand_mod = phrase_flags(cand_text, MODIFIERS)

    movement_overlap = len(query_moves & cand_moves)
    implement_overlap = len(query_impl & cand_impl)
    modifier_overlap = len(query_mod & cand_mod)

    penalty = 0.0
    reasons = []

    if query_moves and not movement_overlap:
        penalty += 0.35
        reasons.append("main movement mismatch")
    elif movement_overlap:
        reasons.append("movement matches")

    if query_impl and not implement_overlap:
        penalty += 0.20
        reasons.append("implement mismatch")
    elif implement_overlap:
        reasons.append("implement matches")

    missing_modifiers = sorted(query_mod - cand_mod)
    if missing_modifiers:
        penalty += min(0.20, 0.08 * len(missing_modifiers))
        reasons.append("missing modifiers: " + ", ".join(missing_modifiers))
    elif modifier_overlap:
        reasons.append("key modifiers match")

    base = 0.52 * ratio + 0.48 * token_jaccard
    boost = 0.08 * movement_overlap + 0.05 * implement_overlap + 0.03 * modifier_overlap
    final = max(0.0, min(1.0, base + boost - penalty))

    return {
        "exercise_id": model.get("id"),
        "name": model.get("name") or model.get("name_default_lng") or model.get("name_second_lng") or "",
        "name_alt": model.get("name_default_lng") if model.get("name_default_lng") != model.get("name") else model.get("name_second_lng"),
        "type": model.get("type"),
        "primary_unit": model.get("primary_unit"),
        "secondary_unit": model.get("secondary_unit"),
        "score": round(final, 4),
        "ratio": round(ratio, 4),
        "token_jaccard": round(token_jaccard, 4),
        "movement_overlap": sorted(query_moves & cand_moves),
        "implement_overlap": sorted(query_impl & cand_impl),
        "modifier_overlap": sorted(query_mod & cand_mod),
        "missing_modifiers": missing_modifiers,
        "reasons": reasons,
    }


def library_models(payload):
    if isinstance(payload, dict) and "models" in payload:
        return payload["models"]
    if isinstance(payload, dict) and "data" in payload and isinstance(payload["data"], dict):
        return payload["data"].get("models", [])
    raise SystemExit("Library JSON must contain .models or .data.models")


def decision(top):
    if not top:
        return {"action": "manual", "reason": "no candidates"}
    if top["score"] >= 0.78 and "main movement mismatch" not in top["reasons"] and "implement mismatch" not in top["reasons"]:
        return {"action": "reuse_library", "reason": "high-confidence semantic match"}
    if top["score"] >= 0.66 and not top["missing_modifiers"] and "main movement mismatch" not in top["reasons"]:
        return {"action": "review_top_candidate", "reason": "close match, verify variation manually"}
    return {"action": "manual", "reason": "best candidate changes a key component or confidence is too low"}


def main():
    parser = argparse.ArgumentParser(description="Rank Trenda library exercises against a BTWB or free-text source exercise.")
    parser.add_argument("--library", required=True, help="Path to Trenda library JSON, usually from get-exercises-by-ids with empty ids body")
    parser.add_argument("--query", required=True, help="Source exercise text")
    parser.add_argument("--limit", type=int, default=8, help="How many candidates to return")
    args = parser.parse_args()

    payload = load_json(args.library)
    models = library_models(payload)
    ranked = [score_candidate(args.query, model) for model in models]
    ranked.sort(key=lambda item: (-item["score"], -item["ratio"], item["name"]))
    top = ranked[: max(1, args.limit)]

    result = {
        "query": args.query,
        "normalized_query": canon_phrase(args.query),
        "library_size": len(models),
        "decision": decision(top[0] if top else None),
        "candidates": top,
    }
    json.dump(result, sys.stdout, ensure_ascii=True, indent=2)
    sys.stdout.write("\n")


if __name__ == "__main__":
    main()
