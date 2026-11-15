package com.example.service;

import com.example.lib.Greeter;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;

/** Serves the lib module's greeting over HTTP once the app runs. */
@RestController
public class HelloController {
    private final Greeter greeter = new Greeter();

    @GetMapping("/")
    public String index() {
        return greeter.greet("GoIDEA");
    }
}
