"""Threat intelligence feed parsers. CPU-bound - run in ProcessPoolExecutor."""
import json
import re
import defusedxml.ElementTree as ET
from dataclasses import dataclass, field
from typing import List, Optional


@dataclass(slots=True)
class ThreatIndicator:
    indicator_type: str
    value: str
    confidence: int = 50
    severity: str = "medium"


def parse_stix_indicators(stix_json: str) -> List[ThreatIndicator]:
    """Parse STIX 2.x bundle JSON for indicators."""
    indicators = []
    try:
        bundle = json.loads(stix_json)
        objects = bundle.get("objects", [])
        for obj in objects:
            if obj.get("type") != "indicator":
                continue
            pattern = obj.get("pattern", "")
            confidence = obj.get("confidence", 50)
            severity = "high" if confidence > 75 else "medium" if confidence > 40 else "low"
            # Extract IP patterns
            for m in re.finditer(r"ipv4-addr:value\s*=\s*'([^']+)'", pattern):
                indicators.append(ThreatIndicator("ip", m.group(1), confidence, severity))
            # Extract domain patterns
            for m in re.finditer(r"domain-name:value\s*=\s*'([^']+)'", pattern):
                indicators.append(ThreatIndicator("domain", m.group(1), confidence, severity))
            # Extract URL patterns
            for m in re.finditer(r"url:value\s*=\s*'([^']+)'", pattern):
                indicators.append(ThreatIndicator("url", m.group(1), confidence, severity))
            # Extract file hash patterns
            for m in re.finditer(r"file:hashes\.'[^']+'\s*=\s*'([^']+)'", pattern):
                indicators.append(ThreatIndicator("hash", m.group(1), confidence, severity))
    except (json.JSONDecodeError, KeyError):
        pass
    return indicators


def parse_openioc_indicators(xml_str: str) -> List[ThreatIndicator]:
    """Parse OpenIOC 1.1 XML for indicators."""
    indicators = []
    try:
        root = ET.fromstring(xml_str)
        ns = {"ioc": "http://schemas.mandiant.com/2010/ioc"}
        for item in root.iter("IndicatorItem"):
            context = item.find("Context", ns) or item.find("context")
            content = item.find("Content", ns) or item.find("content")
            if context is not None and content is not None:
                context_type = context.get("document", "")
                value = content.text or ""
                if "ip" in context_type.lower():
                    indicators.append(ThreatIndicator("ip", value))
                elif "domain" in context_type.lower():
                    indicators.append(ThreatIndicator("domain", value))
                elif "hash" in context_type.lower():
                    indicators.append(ThreatIndicator("hash", value))
    except ET.ParseError:
        pass
    return indicators


def parse_misp_event(misp_json: str) -> List[ThreatIndicator]:
    """Parse MISP JSON event for indicators."""
    indicators = []
    type_map = {
        "ip-src": "ip", "ip-dst": "ip", "ip-src|port": "ip", "ip-dst|port": "ip",
        "domain": "domain", "hostname": "domain",
        "url": "url", "uri": "url",
        "md5": "hash", "sha1": "hash", "sha256": "hash",
    }
    try:
        event = json.loads(misp_json)
        attributes = event.get("Event", {}).get("Attribute", [])
        for attr in attributes:
            attr_type = attr.get("type", "")
            ind_type = type_map.get(attr_type)
            if ind_type:
                value = attr.get("value", "").split("|")[0]
                indicators.append(ThreatIndicator(ind_type, value))
    except (json.JSONDecodeError, KeyError):
        pass
    return indicators
