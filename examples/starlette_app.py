from starlette.applications import Starlette
from starlette.responses import JSONResponse
from starlette.routing import Route
import asyncio

async def homepage(request):
    return JSONResponse({"message": "Hello from Starlette!"})

async def sleep_endpoint(request):
    seconds = int(request.path_params.get('seconds', 1))
    if seconds > 10:
        seconds = 10  # Cap at 10 seconds for safety
    await asyncio.sleep(seconds)
    return JSONResponse({"message": f"Slept for {seconds} seconds"})

routes = [
    Route("/", homepage),
    Route("/sleep/{seconds:int}", sleep_endpoint)
]

app = Starlette(debug=True, routes=routes)