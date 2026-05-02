"""OpenTelemetry tracing configuration for Nest API."""
import os

from opentelemetry import trace
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor, SimpleSpanProcessor, SpanExporter, SpanExportResult
from quart import Quart

from opentelemetry.instrumentation.quart import QuartInstrumentor


def configure_telemetry(app: Quart) -> None:
    """Configure OpenTelemetry tracing for the Nest API.

    Service name: "nest-api"
    Service version: APP_VERSION env var (default: "0.0.0")

    If OTEL_EXPORTER_OTLP_ENDPOINT is set:
        - Configures BatchSpanProcessor with OTLPGrpcExporter
    Otherwise:
        - Uses a no-op exporter (zero overhead)

    Instruments Quart via QuartInstrumentor.
    """
    service_version = os.getenv("APP_VERSION", "0.0.0")

    resource = Resource.create(
        {
            "service.name": "nest-api",
            "service.version": service_version,
        }
    )

    provider = TracerProvider(resource=resource)

    otlp_endpoint = os.getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
    if otlp_endpoint:
        from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter

        exporter = OTLPSpanExporter(endpoint=otlp_endpoint)
        provider.add_span_processor(BatchSpanProcessor(exporter))
    else:
        # No-op path: discard all spans — zero overhead when OTLP not configured
        class _NoopExporter(SpanExporter):
            """Discards all spans — zero overhead placeholder."""

            def export(self, spans):  # type: ignore[override]
                return SpanExportResult.SUCCESS

            def shutdown(self) -> None:  # pragma: no cover
                pass

        provider.add_span_processor(SimpleSpanProcessor(_NoopExporter()))

    trace.set_tracer_provider(provider)

    QuartInstrumentor().instrument_app(app)
