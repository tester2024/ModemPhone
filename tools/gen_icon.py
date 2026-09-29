"""Generates the ModemPhone logo at every size the app needs.

Drawn at 8x and downsampled so the edges stay clean, then written as a
multi-resolution Windows .ico plus the PNGs used for the tray and window.
"""
import os
from PIL import Image, ImageDraw

S = 1024          # working size
OUT = r"G:\modem\assets\icons"

BG_TOP = (0x17, 0x2A, 0x45)      # deep navy
BG_BOT = (0x0A, 0x0E, 0x14)      # near black
ENV = (0xFF, 0xFF, 0xFF)         # white envelope
ENV_SHADE = (0xC7, 0xDB, 0xF5)   # envelope flap
ACCENT = (0x58, 0xA6, 0xFF)      # signal bars
ROUND = int(S * 0.235)            # corner radius


def lerp(a, b, t):
    return tuple(int(a[i] + (b[i] - a[i]) * t) for i in range(3))


def gradient(size):
    """Vertical navy-to-black gradient."""
    img = Image.new("RGB", (1, size))
    d = ImageDraw.Draw(img)
    for y in range(size):
        d.point((0, y), fill=lerp(BG_TOP, BG_BOT, y / max(1, size - 1)))
    return img.resize((size, size), Image.BILINEAR)


def draw_logo():
    img = gradient(S).convert("RGBA")
    d = ImageDraw.Draw(img)

    # Rounded tile, so the icon reads as an app rather than a photo.
    mask = Image.new("L", (S, S), 0)
    ImageDraw.Draw(mask).rounded_rectangle([0, 0, S - 1, S - 1], radius=ROUND, fill=255)
    img.putalpha(mask)

    d = ImageDraw.Draw(img)

    # Envelope body, centred and slightly above the middle to leave room for
    # the signal bars.
    ex0, ex1 = int(S * 0.175), int(S * 0.825)
    ey0, ey1 = int(S * 0.275), int(S * 0.685)
    r = int(S * 0.045)
    d.rounded_rectangle([ex0, ey0, ex1, ey1], radius=r, fill=ENV)

    # Flap: a triangle from the top corners down to the middle. Cutting it with
    # the background colour reads as the crease of a closed envelope.
    d.polygon(
        [(ex0 + r, ey0 + 4), (ex1 - r, ey0 + 4), ((ex0 + ex1) // 2, int(S * 0.545))],
        fill=lerp(BG_TOP, BG_BOT, 0.45),
    )
    # A lighter crease line on top of the flap, so it stays visible against
    # the dark triangle.
    d.line(
        [(ex0 + r, ey0 + 4), ((ex0 + ex1) // 2, int(S * 0.545)), (ex1 - r, ey0 + 4)],
        fill=ENV_SHADE,
        width=int(S * 0.017),
        joint="curve",
    )

    # Signal bars bottom-right: this is a modem, not just a letter.
    bx = int(S * 0.635)
    base = int(S * 0.845)
    bw = int(S * 0.052)
    gap = int(S * 0.028)
    for i, hh in enumerate((0.055, 0.095, 0.135)):
        x0 = bx + i * (bw + gap)
        d.rounded_rectangle(
            [x0, base - int(S * hh), x0 + bw, base],
            radius=int(bw * 0.34),
            fill=ACCENT,
        )

    return img


def main():
    os.makedirs(OUT, exist_ok=True)
    logo = draw_logo()

    # The tray wants a crisp square PNG; Windows scales it itself.
    logo.resize((256, 256), Image.LANCZOS).save(os.path.join(OUT, "tray.png"))

    # A square master for the window icon.
    logo.resize((512, 512), Image.LANCZOS).save(os.path.join(OUT, "app.png"))

    # Multi-resolution .ico, which is what a Windows executable wants.
    ico_path = os.path.join(OUT, "app.ico")
    sizes = [(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)]
    logo.save(ico_path, format="ICO", sizes=sizes, append_images=[])

    # A 16px preview so the small-size legibility can actually be judged.
    logo.resize((16, 16), Image.LANCZOS).resize((160, 160), Image.NEAREST).save(
        os.path.join(OUT, "_preview16.png")
    )
    logo.resize((32, 32), Image.LANCZOS).resize((160, 160), Image.NEAREST).save(
        os.path.join(OUT, "_preview32.png")
    )

    for f in sorted(os.listdir(OUT)):
        p = os.path.join(OUT, f)
        print(f"{f:20} {os.path.getsize(p):>8} bytes")


if __name__ == "__main__":
    main()
