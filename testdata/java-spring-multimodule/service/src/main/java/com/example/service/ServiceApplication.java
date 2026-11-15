package com.example.service;

import com.example.lib.Greeter;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

/** The Spring Boot main class of the nested service module. */
@SpringBootApplication
public class ServiceApplication {
    public static void main(String[] args) {
        Greeter greeter = new Greeter();
        SpringApplication.run(ServiceApplication.class, args);
        // Printed once Spring is up: it proves the nested module's runtime
        // classpath carried the sibling lib module too.
        System.out.println("GREETING:" + greeter.greet("GoIDEA"));
    }
}
