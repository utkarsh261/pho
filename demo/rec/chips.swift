// Draws the overlay PNGs with CoreText, no browser: key chips (JetBrains Mono
// on a dark keycap) and commentary labels (system sans, so they never read as
// pho's own UI). Sizes are for the 4K frame.
//
// usage: swift chips.swift <out_dir> <spec.json>
// spec.json: {"keys": {"name": "text", ...}, "labels": {"name": "text", ...}, "opts": {"pill": "1"}}
// With opts.pill the labels sit on a dark pill, for takes that zoom away from
// the status bar.
import AppKit
import Foundation

let args = CommandLine.arguments
let outDir = URL(fileURLWithPath: args[1])
let spec = try JSONSerialization.jsonObject(with: Data(contentsOf: URL(fileURLWithPath: args[2]))) as! [String: [String: String]]
try FileManager.default.createDirectory(at: outDir, withIntermediateDirectories: true)

func color(_ hex: UInt32, _ a: CGFloat = 1) -> NSColor {
    NSColor(srgbRed: CGFloat((hex >> 16) & 0xff) / 255, green: CGFloat((hex >> 8) & 0xff) / 255,
            blue: CGFloat(hex & 0xff) / 255, alpha: a)
}

func render(_ name: String, width: Int, height: Int, draw: (CGContext) -> Void) {
    let rep = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: width, pixelsHigh: height, bitsPerSample: 8,
                               samplesPerPixel: 4, hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB,
                               bytesPerRow: 0, bitsPerPixel: 0)!
    NSGraphicsContext.saveGraphicsState()
    NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: rep)
    draw(NSGraphicsContext.current!.cgContext)
    NSGraphicsContext.restoreGraphicsState()
    try! rep.representation(using: .png, properties: [:])!.write(to: outDir.appendingPathComponent("\(name).png"))
}

let mono = NSFont(name: "JetBrainsMono-Medium", size: 27) ?? NSFont.monospacedSystemFont(ofSize: 27, weight: .medium)
let sans = NSFont.systemFont(ofSize: 31, weight: .medium)

for (name, text) in spec["keys"] ?? [:] {
    let str = NSAttributedString(string: text, attributes: [.font: mono, .foregroundColor: color(0xECEDEE)])
    let ts = str.size()
    let h = 38, w = max(Int(ts.width.rounded(.up)) + 30, h)
    render("key-\(name)", width: w + 4, height: h + 4) { _ in
        let r = NSRect(x: 2, y: 2, width: w, height: h)
        let path = NSBezierPath(roundedRect: r, xRadius: 9, yRadius: 9)
        color(0x2B2D30, 0.96).setFill(); path.fill()
        color(0x5A5E66).setStroke(); path.lineWidth = 2; path.stroke()
        str.draw(at: NSPoint(x: r.midX - ts.width / 2, y: r.midY - ts.height / 2))
    }
}

let pill = spec["opts"]?["pill"] != nil
for (name, text) in spec["labels"] ?? [:] {
    let shadow = NSShadow()
    shadow.shadowColor = color(0x000000, 0.55); shadow.shadowBlurRadius = 6; shadow.shadowOffset = .zero
    let str = NSAttributedString(string: text, attributes: [.font: sans, .foregroundColor: color(0xC9C1F2), .shadow: shadow])
    let ts = str.size()
    let pad = pill ? 22 : 8
    render("label-\(name)", width: Int(ts.width.rounded(.up)) + 2 * pad, height: pill ? 52 : 42) { _ in
        let h: CGFloat = pill ? 52 : 42
        if pill {
            let path = NSBezierPath(roundedRect: NSRect(x: 1, y: 1, width: ts.width + CGFloat(2 * pad) - 2, height: h - 2),
                                    xRadius: 25, yRadius: 25)
            color(0x16171A, 0.92).setFill(); path.fill()
            color(0x4A4560).setStroke(); path.lineWidth = 2; path.stroke()
        }
        str.draw(at: NSPoint(x: CGFloat(pad), y: (h - ts.height) / 2))
    }
}
print("chips -> \(outDir.path)")
